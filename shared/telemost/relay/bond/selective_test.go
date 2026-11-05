// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package bond

import (
	"crypto/sha256"
	"testing"
	"time"
)

func selectiveSetup(t *testing.T) *pairedTestNetwork {
	t.Helper()
	key := sha256.Sum256([]byte("один основной и облегчённый резерв"))
	n := &pairedTestNetwork{a: NewSelective(key, true, nil), b: NewSelective(key, false, nil)}
	for i := 0; i < SelectiveLanes; i++ {
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
		for i := 0; i < SelectiveLanes; i++ {
			close(n.aa[i].stop)
			close(n.bb[i].stop)
		}
	})
	for _, tr := range []*Transport{n.a, n.b} {
		select {
		case <-tr.Ready():
		case <-time.After(4 * time.Second):
			t.Fatal("пара не готова")
		}
	}
	return n
}
func TestЗдоровыйОсновнойНеКопируетсяВРезерв(t *testing.T) {
	n := selectiveSetup(t)
	pairedTransfer(t, n.a, n.b, 256)
	pairedWait(t, func() bool { n.a.mu.Lock(); defer n.a.mu.Unlock(); return len(n.a.outbound) == 0 }, "не освобождён буфер")
	if s := n.a.Stats(); s[0].Sent == 0 || s[1].Sent != 0 {
		t.Fatal("резерв дублирует здоровый поток вместо восстановления пропусков")
	}
}
func TestВыборочноеВосстановлениеПервогоИПоследнегоБлока(t *testing.T) {
	for _, seq := range []uint64{1, 128} {
		t.Run(string(rune('А'+seq%32)), func(t *testing.T) {
			n := selectiveSetup(t)
			n.aa[0].onlySequence.Store(seq)
			pairedTransfer(t, n.a, n.b, 128)
			if s := n.b.Stats(); s[1].Recovered == 0 {
				t.Fatal("пропуск не восстановлен резервом")
			}
		})
	}
}
func TestСкрытыйОбрывОсновногоНеТеряетПоток(t *testing.T) {
	n := selectiveSetup(t)
	n.aa[0].blackhole.Store(true)
	n.bb[0].blackhole.Store(true)
	pairedTransfer(t, n.a, n.b, 1024)
}
func TestЯвныйОбрывОсновногоНеЗакрываетПару(t *testing.T) {
	n := selectiveSetup(t)
	n.a.Detach(0, n.aa[0])
	n.b.Detach(0, n.bb[0])
	pairedTransfer(t, n.a, n.b, 256)
}
func TestЗастывшийОсновнойНеОстанавливаетВыборочныйРезерв(t *testing.T) {
	n := selectiveSetup(t)
	n.aa[0].delayNanos.Store(int64(time.Minute))
	pairedTransfer(t, n.a, n.b, 512)
}
func TestПрофильРезерваДействительноНижеОсновного(t *testing.T) {
	p := DefaultSelectiveProfile()
	if !p.Valid() {
		t.Fatal("неверные настройки по умолчанию")
	}
	if p.ReserveFPS*p.ReserveBatch >= p.MainFPS*p.MainBatch {
		t.Fatal("резерв не облегчён")
	}
}

func TestРезервНеПодмешиваетНовыеБлокиПослеПростоя(t *testing.T) {
	n := selectiveSetup(t)
	n.a.mu.Lock()
	n.a.lanes[0].lastReceipt = time.Now().Add(-5 * time.Second)
	n.a.mu.Unlock()
	n.aa[0].delayNanos.Store(int64(2 * time.Millisecond))
	pairedTransfer(t, n.a, n.b, 128)
	if n.a.Stats()[1].Sent != 0 {
		t.Fatal("заполненная очередь после простоя стала распределением новых блоков на резерв")
	}
}

func TestЗапоздалаяЭпохаРезерваНеСбрасываетПроверенныйОсновной(t *testing.T) {
	key := sha256.Sum256([]byte("запоздалая эпоха"))
	client := NewSelective(key, false, nil)
	old := NewSelective(key, true, nil)
	defer client.Close()
	defer old.Close()
	x, y := pair(0)
	defer close(x.stop)
	defer close(y.stop)
	client.Attach(1, y)
	client.mu.Lock()
	client.epoch = Epoch{1}
	client.primaryEpochPinned = true
	client.lanes[0].verified = true
	client.mu.Unlock()
	old.mu.Lock()
	p := old.encode(kindChallenge, 1, 0, make([]byte, 32))
	old.mu.Unlock()
	client.Handle(1, y, p)
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.epoch != (Epoch{1}) || !client.lanes[0].verified {
		t.Fatal("запоздалый резерв отменил проверенную основную эпоху")
	}
	if client.err != nil {
		t.Fatal("безопасный запоздалый вызов закрыл начальную попытку")
	}
}

func TestОкноВыборочнойПарыОграничиваетЗадержку(t *testing.T) {
	tr := NewSelective([32]byte{1}, false, nil)
	defer tr.Close()
	for mode, want := range map[Mode]uint64{Speed: 64, Balanced: 32, Stability: 16} {
		tr.mode = mode
		if tr.selectiveWindow() != want {
			t.Fatal("неверное окно основной сессии")
		}
	}
}

func TestЗапоздалаяЭпохаРезерваПослеГотовностиНеЗакрываетSOCKS(t *testing.T) {
	key := sha256.Sum256([]byte("запоздалая эпоха"))
	client := NewSelective(key, false, nil)
	old := NewSelective(key, true, nil)
	defer client.Close()
	defer old.Close()
	x, y := pair(0)
	defer close(x.stop)
	defer close(y.stop)
	client.Attach(1, y)
	client.mu.Lock()
	client.epoch = Epoch{1}
	client.primaryEpochPinned = true
	client.everReady = true
	client.lanes[0].verified = true
	client.mu.Unlock()
	old.mu.Lock()
	p := old.encode(kindChallenge, 1, 0, make([]byte, 32))
	old.mu.Unlock()
	client.Handle(1, y, p)
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.epoch != (Epoch{1}) || !client.lanes[0].verified {
		t.Fatal("запоздалый резерв отменил проверенную основную эпоху")
	}
	if client.err != nil {
		t.Fatal("безопасный запоздалый вызов закрыл начальную попытку")
	}
}
