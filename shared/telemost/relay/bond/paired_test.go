// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package bond

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"whitelist-bypass/relay/tunnel"
)

// Испытательный провод теряет уже целые блоки надёжного транспорта: это более
// жёсткий сбой, чем обычная потеря RTP-пакета, которую сначала исправляет KCP.
type impairedWire struct {
	*fakeWire
	drop         atomic.Bool
	delayNanos   atomic.Int64
	onlySequence atomic.Uint64
	blackhole    atomic.Bool
}

func (f *impairedWire) SendData(p []byte) {
	if f.blackhole.Load() {
		return
	}
	data := false
	var seq uint64
	tunnel.DecodeFrames(p, func(_ uint32, _ byte, b []byte) {
		if len(b) >= header+signature && b[4] == kindData {
			data = true
			seq = binary.BigEndian.Uint64(b[22:30])
		}
	})
	if data && (f.drop.Load() || (f.onlySequence.Load() != 0 && seq == f.onlySequence.Load())) {
		return
	}
	if data && f.delayNanos.Load() > 0 {
		timer := time.NewTimer(time.Duration(f.delayNanos.Load()))
		defer timer.Stop()
		select {
		case <-f.stop:
			return
		case <-timer.C:
		}
	}
	f.fakeWire.SendData(p)
}

type pairedTestNetwork struct {
	a, b   *Transport
	aa, bb [PairedLanes]*impairedWire
}

func pairedSetup(t *testing.T) *pairedTestNetwork {
	t.Helper()
	key := sha256.Sum256([]byte("две основные и две резервные"))
	n := &pairedTestNetwork{a: NewPaired(key, true, nil, 50*time.Millisecond), b: NewPaired(key, false, nil, 50*time.Millisecond)}
	for i := 0; i < PairedLanes; i++ {
		i := i
		x, y := pair(0)
		n.aa[i], n.bb[i] = &impairedWire{fakeWire: x}, &impairedWire{fakeWire: y}
		x.SetOnData(func(p []byte) { n.a.Handle(i, n.aa[i], p) })
		y.SetOnData(func(p []byte) { n.b.Handle(i, n.bb[i], p) })
		n.a.Attach(i, n.aa[i])
		n.b.Attach(i, n.bb[i])
	}
	t.Cleanup(func() {
		n.a.Close()
		n.b.Close()
		for i := 0; i < PairedLanes; i++ {
			close(n.aa[i].stop)
			close(n.bb[i].stop)
		}
	})
	for _, tr := range []*Transport{n.a, n.b} {
		select {
		case <-tr.Ready():
		case <-time.After(4 * time.Second):
			t.Fatal("четвёрка не прошла проверку")
		}
	}
	return n
}
func pairedTransfer(t *testing.T, a, b *Transport, count int) {
	t.Helper()
	const size = 8192
	want := make([]byte, count*size)
	for i := 0; i < count; i++ {
		for j := 0; j < size; j++ {
			want[i*size+j] = byte(i + j)
		}
		binary.BigEndian.PutUint32(want[i*size:], uint32(i))
	}
	var mu sync.Mutex
	var got bytes.Buffer
	done := make(chan struct{})
	b.SetOnData(func(p []byte) {
		mu.Lock()
		defer mu.Unlock()
		got.Write(p)
		if got.Len() == len(want) {
			close(done)
		}
	})
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		for i := 0; i < count; i++ {
			a.SendData(want[i*size : (i+1)*size])
		}
	}()
	select {
	case <-done:
	case <-time.After(12 * time.Second):
		t.Fatalf("поток не восстановлен: %v / %v", a.Err(), b.Err())
	}
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("отправитель завис")
	}
	mu.Lock()
	equal := bytes.Equal(got.Bytes(), want)
	mu.Unlock()
	if !equal {
		t.Fatal("байты или порядок изменились")
	}
	if a.Err() != nil || b.Err() != nil {
		t.Fatal("переподключение вместо восстановления")
	}
}
func pairedWait(t *testing.T, fn func() bool, msg string) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if fn() {
			return
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal(msg)
		}
	}
}
func TestРезервПередаётКопииДажеПриУспехеОсновных(t *testing.T) {
	n := pairedSetup(t)
	pairedTransfer(t, n.a, n.b, 64)
	pairedWait(t, func() bool {
		s := n.a.Stats()
		r := n.b.Stats()
		return s[2].Sent > 0 && s[3].Sent > 0 && r[2].Duplicates+r[3].Duplicates > 0
	}, "резервы не передали задержанные копии")
	for _, s := range n.a.Stats() {
		if s.Sent == 0 {
			t.Fatalf("сессия %d не участвовала", s.Lane)
		}
	}
}
func TestРезервВосстанавливаетПотерюБлоковБезОтключения(t *testing.T) {
	n := pairedSetup(t)
	n.aa[0].drop.Store(true)
	n.aa[1].drop.Store(true)
	pairedTransfer(t, n.a, n.b, 512)
	s := n.b.Stats()
	if s[2].Recovered+s[3].Recovered == 0 {
		t.Fatal("нет восстановления с резервов")
	}
}
func TestПросевшийОсновнойБитрейтНеТормозитРезерв(t *testing.T) {
	n := pairedSetup(t)
	n.aa[0].delayNanos.Store(int64(300 * time.Millisecond))
	n.aa[1].delayNanos.Store(int64(300 * time.Millisecond))
	pairedTransfer(t, n.a, n.b, 256)
	if s := n.b.Stats(); s[2].Recovered+s[3].Recovered == 0 {
		t.Fatal("резерв не обогнал задержанные основные блоки")
	}
}
func TestПотеряБлокаПоОбоимПарнымПутям(t *testing.T) {
	n := pairedSetup(t)
	n.aa[0].onlySequence.Store(1)
	n.aa[2].onlySequence.Store(1)
	pairedTransfer(t, n.a, n.b, 256)
}
func TestОтсоединениеОсновныхНеЗакрываетПоток(t *testing.T) {
	n := pairedSetup(t)
	for i := 0; i < 2; i++ {
		n.a.Detach(i, n.aa[i])
		n.b.Detach(i, n.bb[i])
	}
	pairedTransfer(t, n.a, n.b, 256)
}
func TestПовторноеПодключениеСессииСохраняетЭпоху(t *testing.T) {
	n := pairedSetup(t)
	n.a.mu.Lock()
	epoch := n.a.epoch
	n.a.mu.Unlock()
	n.a.Detach(0, n.aa[0])
	n.b.Detach(0, n.bb[0])
	n.a.Attach(0, n.aa[0])
	n.b.Attach(0, n.bb[0])
	pairedWait(t, func() bool { return n.a.Stats()[0].Verified && n.b.Stats()[0].Verified }, "повторный канал не прошёл проверку")
	pairedTransfer(t, n.a, n.b, 64)
	n.a.mu.Lock()
	same := epoch == n.a.epoch
	n.a.mu.Unlock()
	if !same {
		t.Fatal("подменена эпоха потока")
	}
}
func TestБуферКопийОсвобождаетсяПослеПодтверждения(t *testing.T) {
	n := pairedSetup(t)
	pairedTransfer(t, n.a, n.b, 1024)
	pairedWait(t, func() bool { n.a.mu.Lock(); defer n.a.mu.Unlock(); return len(n.a.outbound) == 0 }, "буфер подтверждённых копий не освобождён")
}
func TestДвунаправленныеПотериВЧетырёхСессиях(t *testing.T) {
	n := pairedSetup(t)
	n.aa[0].drop.Store(true)
	n.aa[1].drop.Store(true)
	n.bb[0].drop.Store(true)
	n.bb[1].drop.Store(true)
	for _, side := range []struct{ a, b *Transport }{{n.a, n.b}, {n.b, n.a}} {
		t.Run(fmt.Sprint(side.a.server), func(t *testing.T) { pairedTransfer(t, side.a, side.b, 512) })
	}
}

func TestСкрытаяПотеряОсновныхНеТеряетСквозныеПодтверждения(t *testing.T) {
	n := pairedSetup(t)
	for i := 0; i < 2; i++ {
		n.aa[i].blackhole.Store(true)
		n.bb[i].blackhole.Store(true)
	}
	pairedTransfer(t, n.a, n.b, 1024)
}
func TestЗастывшийОсновнойНеЗаполняетБуферПодтверждённыхКопий(t *testing.T) {
	n := pairedSetup(t)
	for i := 0; i < 2; i++ {
		n.aa[i].delayNanos.Store(int64(time.Minute))
	}
	pairedTransfer(t, n.a, n.b, 1024)
	pairedWait(t, func() bool { n.a.mu.Lock(); defer n.a.mu.Unlock(); return len(n.a.outbound) == 0 }, "доставленные копии остались в буфере")
}
