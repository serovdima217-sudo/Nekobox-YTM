// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package dup

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestЗадержкаИВосстановление(t *testing.T) {
	primary := make(chan []byte, 64)
	secondary := make(chan []byte, 64)
	c := New(40*time.Millisecond, 128, 1<<20, func(b []byte) bool { primary <- b; return true }, func(b []byte) bool { secondary <- b; return true })
	defer c.Close()
	r := New(time.Millisecond, 1, 128, func([]byte) bool { return false }, func([]byte) bool { return false })
	defer r.Close()
	start := time.Now()
	for i := 0; i < 20; i++ {
		p := []byte(fmt.Sprintf("блок-%02d", i))
		c.Send(p)
		p[0] = 0
	}
	received := map[string]bool{}
	for i := 0; i < 20; i++ {
		b := <-primary
		if i%3 != 0 {
			p, ok := r.Receive(b, false)
			if !ok {
				t.Fatal("первая копия отклонена")
			}
			received[string(p)] = true
		}
	}
	if len(secondary) != 0 {
		t.Fatal("копия отправлена раньше задержки")
	}
	timeout := time.After(time.Second)
	for i := 0; i < 20; i++ {
		select {
		case b := <-secondary:
			if time.Since(start) < 40*time.Millisecond {
				t.Fatal("нарушена задержка")
			}
			p, ok := r.Receive(b, true)
			if ok {
				if received[string(p)] {
					t.Fatal("повтор доставлен приложению")
				}
				received[string(p)] = true
			}
		case <-timeout:
			t.Fatal("копии не пришли")
		}
	}
	if len(received) != 20 || r.Stats().Recovered != 7 || r.Stats().Duplicates != 13 {
		t.Fatal("потерянные данные не восстановлены или неверная дедупликация")
	}
}
func TestОграничениеПамяти(t *testing.T) {
	c := New(2*time.Second, 10, 400, func([]byte) bool { return true }, func([]byte) bool { return false })
	defer c.Close()
	for i := 0; i < 100; i++ {
		c.Send(make([]byte, 100))
	}
	s := c.Stats()
	if s.QueuedBytes > 400 || s.QueueDrops < 97 {
		t.Fatal("лимит очереди нарушен")
	}
	c.Close()
	if c.Stats().QueuedBytes != 0 {
		t.Fatal("очередь не очищена")
	}
}
func TestКонкурентность(t *testing.T) {
	c := New(time.Millisecond, 128, 1<<20, func([]byte) bool { return true }, func([]byte) bool { return true })
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				c.Send([]byte{1, 2, 3})
				c.Stats()
			}
			c.Close()
		}()
	}
	wg.Wait()
	if c.Send([]byte{1}) {
		t.Fatal("отправка после закрытия принята")
	}
}
func TestПовторыИПерестановка(t *testing.T) {
	var packets [][]byte
	c := New(time.Second, 100, 1<<20, func(p []byte) bool { packets = append(packets, p); return true }, func([]byte) bool { return true })
	defer c.Close()
	for i := 0; i < 20; i++ {
		c.Send([]byte{byte(i)})
	}
	for i := 19; i >= 0; i-- {
		p, ok := c.Receive(packets[i], true)
		if !ok || !bytes.Equal(p, []byte{byte(i)}) {
			t.Fatal("перестановка потеряла пакет")
		}
		if _, ok = c.Receive(packets[i], false); ok {
			t.Fatal("дубль принят")
		}
	}
	if _, ok := c.Receive([]byte("неверный заголовок"), false); ok {
		t.Fatal("неверный заголовок принят")
	}
}
