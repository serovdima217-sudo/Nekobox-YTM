// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package bond

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"testing"
	"time"
	"whitelist-bypass/relay/tunnel"
)

type fakeWire struct {
	mu    sync.Mutex
	data  func([]byte)
	close func()
	peer  *fakeWire
	q     chan []byte
	stop  chan struct{}
	delay time.Duration
}

func (f *fakeWire) SendData(p []byte) {
	if !tunnel.LooksLikeRelayFrame(p) || binary.BigEndian.Uint32(p[4:8]) != LaneConnID || p[8] != tunnel.MsgData {
		panic("пакет объединения должен идти через надёжную ветку KCP")
	}

	p = append([]byte{}, p...)
	select {
	case f.q <- p:
	case <-f.stop:
	}
}
func (f *fakeWire) SetOnData(fn func([]byte)) { f.mu.Lock(); f.data = fn; f.mu.Unlock() }
func (f *fakeWire) SetOnClose(fn func())      { f.mu.Lock(); f.close = fn; f.mu.Unlock() }
func (*fakeWire) Reconfigure(int, int)        {}
func (f *fakeWire) run() {
	for {
		select {
		case <-f.stop:
			return
		case p := <-f.q:
			if f.delay > 0 {
				select {
				case <-f.stop:
					return
				case <-time.After(f.delay):
				}
			}
			f.peer.mu.Lock()
			fn := f.peer.data
			f.peer.mu.Unlock()
			if fn != nil {
				fn(p)
			}
		}
	}
}
func pair(delay time.Duration) (*fakeWire, *fakeWire) {
	a := &fakeWire{q: make(chan []byte, 32), stop: make(chan struct{}), delay: delay}
	b := &fakeWire{q: make(chan []byte, 32), stop: make(chan struct{}), delay: delay}
	a.peer = b
	b.peer = a
	go a.run()
	go b.run()
	return a, b
}
func setup(t *testing.T) (*Transport, *Transport, [Lanes]*fakeWire, [Lanes]*fakeWire) {
	t.Helper()
	key := sha256.Sum256([]byte("испытание десяти каналов"))
	a, b := New(key, true, nil), New(key, false, nil)
	var aa, bb [Lanes]*fakeWire
	for i := 0; i < Lanes; i++ {
		i := i
		aa[i], bb[i] = pair(time.Duration(i%4) * time.Millisecond)
		aa[i].SetOnData(func(p []byte) { a.Handle(i, aa[i], p) })
		bb[i].SetOnData(func(p []byte) { b.Handle(i, bb[i], p) })
		a.Attach(i, aa[i])
		b.Attach(i, bb[i])
	}
	t.Cleanup(func() {
		a.Close()
		b.Close()
		for i := range aa {
			close(aa[i].stop)
			close(bb[i].stop)
		}
	})
	select {
	case <-a.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("сервер не проверил десять каналов")
	}
	select {
	case <-b.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("клиент не проверил десять каналов")
	}
	return a, b, aa, bb
}
func TestДесятьКаналовВосстанавливаютОдинПоток(t *testing.T) {
	a, b, _, _ := setup(t)
	const count = 512
	done := make(chan struct{})
	var mu sync.Mutex
	var got bytes.Buffer
	b.SetOnData(func(p []byte) {
		mu.Lock()
		defer mu.Unlock()
		got.Write(p)
		if got.Len() == count*4096 {
			close(done)
		}
	})
	var want bytes.Buffer
	for i := 0; i < count; i++ {
		p := bytes.Repeat([]byte{byte(i)}, 4096)
		binary.BigEndian.PutUint32(p, uint32(i))
		want.Write(p)
	}
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		for i := 0; i < count; i++ {
			a.SendData(want.Bytes()[i*4096 : (i+1)*4096])
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("поток не восстановлен: %v / %v", a.Err(), b.Err())
	}
	<-sendDone
	mu.Lock()
	equal := bytes.Equal(got.Bytes(), want.Bytes())
	mu.Unlock()
	if !equal {
		t.Fatal("порядок или байты изменились")
	}
	for _, s := range a.Stats() {
		if !s.Verified || s.Sent == 0 {
			t.Fatalf("канал не участвовал: %+v", s)
		}
	}
	for _, s := range b.Stats() {
		if !s.Verified || s.Received == 0 {
			t.Fatalf("канал не участвовал: %+v", s)
		}
	}
}
func TestОдновременнаяПередачаВОбеСтороны(t *testing.T) {
	a, b, _, _ := setup(t)
	da, db := make(chan struct{}), make(chan struct{})
	na, nb := 0, 0
	a.SetOnData(func(p []byte) {
		if !bytes.Equal(p, []byte("клиент")) {
			t.Error("искажён восходящий поток")
		}
		na++
		if na == 256 {
			close(da)
		}
	})
	b.SetOnData(func(p []byte) {
		if !bytes.Equal(p, []byte("сервер")) {
			t.Error("искажён нисходящий поток")
		}
		nb++
		if nb == 256 {
			close(db)
		}
	})
	go func() {
		for i := 0; i < 256; i++ {
			a.SendData([]byte("сервер"))
		}
	}()
	go func() {
		for i := 0; i < 256; i++ {
			b.SendData([]byte("клиент"))
		}
	}()
	for _, ch := range []chan struct{}{da, db} {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatal("двусторонняя передача зависла")
		}
	}
}
func TestПотеряКаналаЗакрываетСессию(t *testing.T) {
	a, b, aa, _ := setup(t)
	closed := make(chan struct{})
	a.SetOnClose(func() { close(closed) })
	a.Detach(3, aa[3])
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("потеря не закрыла TCP-сессию")
	}
	before := a.Stats()
	a.SendData([]byte("нельзя отправлять"))
	after := a.Stats()
	for i := range before {
		if before[i].Sent != after[i].Sent {
			t.Fatal("закрытый поток продолжил передачу")
		}
	}
	b.Close()
}
func TestНевернаяПодписьИСтараяСессияОтклоняются(t *testing.T) {
	a, b, _, bb := setup(t)
	received := make(chan struct{}, 1)
	b.SetOnData(func([]byte) { received <- struct{}{} })
	a.mu.Lock()
	p := a.encode(kindData, 0, 1, []byte("подмена"))
	a.mu.Unlock()
	p[len(p)-1] ^= 1
	b.Handle(0, bb[0], p)
	a.mu.Lock()
	old := a.epoch
	a.epoch[0] ^= 1
	p = a.encode(kindData, 0, 1, []byte("старая сессия"))
	a.epoch = old
	a.mu.Unlock()
	b.Handle(0, bb[0], p)
	select {
	case <-received:
		t.Fatal("подмена или старая сессия доставлена")
	case <-time.After(50 * time.Millisecond):
	}
}
func TestПереполнениеВосстановленияНеПортитПоток(t *testing.T) {
	a, b, _, bb := setup(t)
	a.mu.Lock()
	p := a.encode(kindData, 0, maxFrames+1, []byte("слишком далеко"))
	a.mu.Unlock()
	b.Handle(0, bb[0], p)
	select {
	case <-b.Done():
	case <-time.After(time.Second):
		t.Fatal("переполнение не закрыло поток")
	}
}
func TestБезДесятогоКаналаНетГотовности(t *testing.T) {
	key := sha256.Sum256([]byte("неполная группа"))
	a, b := New(key, true, nil), New(key, false, nil)
	defer a.Close()
	defer b.Close()
	var aa, bb [Lanes - 1]*fakeWire
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
		t.Fatal("неполная группа объявлена готовой")
	case <-time.After(150 * time.Millisecond):
	}
}
func TestКлючЗависитОтПорядкаИУникальности(t *testing.T) {
	links := make([]string, Lanes)
	for i := range links {
		links[i] = "https://telemost.yandex.ru/j/12345" + string(rune('0'+i))
	}
	k, e := GroupKey(links)
	if e != nil {
		t.Fatal(e)
	}
	links[0], links[1] = links[1], links[0]
	k2, e := GroupKey(links)
	if e != nil || k == k2 {
		t.Fatal("порядок потерян")
	}
	links[1] = links[0]
	if _, e = GroupKey(links); e == nil {
		t.Fatal("повтор принят")
	}
}

func TestЗакрытиеМожноПовторитьИзОбработчика(t *testing.T) {
	t1 := New(sha256.Sum256([]byte("закрытие")), false, nil)
	t1.SetOnClose(t1.Close)
	done := make(chan struct{})
	go func() { t1.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("повторное закрытие зависло")
	}
}
