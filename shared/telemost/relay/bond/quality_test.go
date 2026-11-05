// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package bond

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"
	"time"
)

func TestРаспределительПредпочитаетБыструюДоставку(t *testing.T) {
	tr := New(sha256.Sum256([]byte("качество")), false, nil)
	defer tr.Close()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	for i := range tr.lanes {
		l := &tr.lanes[i]
		l.verified = true
		l.sent = 100
		l.delivered = 100
		l.rate = 256 * 1024
		l.rtt = .9
		l.jitter = .2
	}
	tr.lanes[4].rtt = .05
	tr.lanes[4].jitter = .002
	if got := tr.chooseLaneLocked(32768); got != 4 {
		t.Fatalf("выбран плохой канал %d", got+1)
	}
	// Короткая задержка не оправдывает бесконечное накопление очереди.
	tr.lanes[4].sent += 4 << 20
	if got := tr.chooseLaneLocked(32768); got == 4 {
		t.Fatal("переполненный канал продолжил получать блоки")
	}
}
func TestВыборРежимаИПределовОкна(t *testing.T) {
	for _, s := range []string{"скорость", "speed", "баланс", "balanced", "устойчивость", "stable"} {
		if _, e := ParseMode(s); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := ParseMode("случайный"); e == nil {
		t.Fatal("неверный режим принят")
	}
	if !(Speed.window() > Balanced.window() && Balanced.window() > Stability.window()) {
		t.Fatal("режимы не меняют предел очереди")
	}
	a, b, _, _ := setup(t)
	if e := a.SetMode(Speed); e == nil {
		t.Fatal("режим изменён внутри работающей сессии")
	}
	if b.mode != Balanced {
		t.Fatal("по умолчанию нужен баланс")
	}
}
func TestДесятьКаналовСогласуютРежимСкорости(t *testing.T) {
	key := sha256.Sum256([]byte("согласование режима"))
	a, b := New(key, true, nil), New(key, false, nil)
	defer a.Close()
	defer b.Close()
	b.SetMode(Speed)
	var aa, bb [Lanes]*fakeWire
	for i := range aa {
		i := i
		aa[i], bb[i] = pair(0)
		aa[i].SetOnData(func(p []byte) { a.Handle(i, aa[i], p) })
		bb[i].SetOnData(func(p []byte) { b.Handle(i, bb[i], p) })
		a.Attach(i, aa[i])
		b.Attach(i, bb[i])
		defer close(aa[i].stop)
		defer close(bb[i].stop)
	}
	select {
	case <-b.Ready():
	case <-time.After(3 * time.Second):
		t.Fatal("режим не согласован")
	}
	a.mu.Lock()
	mode := a.mode
	a.mu.Unlock()
	if mode != Speed {
		t.Fatal("сервер не принял режим клиента")
	}
}

func TestМалыйЗапросНеЗанижаетПропускнуюСпособность(t *testing.T) {
	tr := New(sha256.Sum256([]byte("малый запрос")), false, nil)
	defer tr.Close()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	l := &tr.lanes[0]
	l.sent = 1000
	l.timeRate = time.Now().Add(-time.Second)
	l.lastReceipt = time.Now().Add(-100 * time.Millisecond)
	var total [8]byte
	binary.BigEndian.PutUint64(total[:], 100)
	tr.receiptLocked(0, 1, total[:])
	if l.rate != 0 {
		t.Fatal("малый TLS-запрос ошибочно принят за измерение ёмкости")
	}
}

func TestРазноеЧислоПодключённыхКаналов(t *testing.T) {
	for _, width := range []int{1, 2, 3, 5, 8, 10} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			key := sha256.Sum256([]byte(fmt.Sprint(width)))
			a, b := New(key, true, nil, width), New(key, false, nil, width)
			defer a.Close()
			defer b.Close()
			wires := make([][2]*fakeWire, width)
			for i := 0; i < width; i++ {
				i := i
				x, y := pair(time.Duration(i%3) * time.Millisecond)
				wires[i] = [2]*fakeWire{x, y}
				x.SetOnData(func(p []byte) { a.Handle(i, x, p) })
				y.SetOnData(func(p []byte) { b.Handle(i, y, p) })
				a.Attach(i, x)
				b.Attach(i, y)
			}
			defer func() {
				for _, p := range wires {
					close(p[0].stop)
					close(p[1].stop)
				}
			}()
			select {
			case <-a.Ready():
			case <-time.After(3 * time.Second):
				t.Fatal("сервер не готов")
			}
			select {
			case <-b.Ready():
			case <-time.After(3 * time.Second):
				t.Fatal("клиент не готов")
			}
			got := make(chan []byte, 64)
			b.SetOnData(func(p []byte) { got <- append([]byte{}, p...) })
			for i := 0; i < 32; i++ {
				a.SendData([]byte(fmt.Sprint(i)))
			}
			for i := 0; i < 32; i++ {
				select {
				case p := <-got:
					if string(p) != fmt.Sprint(i) {
						t.Fatal("нарушен порядок")
					}
				case <-time.After(time.Second):
					t.Fatal("потерян блок")
				}
			}
			if len(a.Stats()) != width || len(b.Stats()) != width {
				t.Fatal("в статистике лишние каналы")
			}
		})
	}
}
