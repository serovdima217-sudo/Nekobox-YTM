// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package rtc

import (
	"bytes"
	"testing"
	"time"
	"whitelist-bypass/relay/tunnel"
)

func TestПакетноеПланированиеСохраняетЧастоту(t *testing.T) {
	for _, step := range []time.Duration{time.Second / 2400, time.Second / 720, 10 * time.Millisecond} {
		tick, burst := pacingBurst(step)
		if burst < 1 || burst > 8 || tick != step*time.Duration(burst) {
			t.Fatal("нарушена средняя частота пакетов")
		}
		if tick < 2*time.Millisecond {
			t.Fatal("слишком частые пробуждения при штатной частоте")
		}
	}
}

func TestНемедленныеACKНеЗадерживаютКрупныйБлок(t *testing.T) {
	log := func(string, ...any) {}
	ta := NewVP8DataTunnelWithQueue(nil, nil, log, 256)
	tb := NewVP8DataTunnelWithQueue(nil, nil, log, 256)
	ma, mb := NewMultiTrackTunnel([]*VP8DataTunnel{ta}), NewMultiTrackTunnel([]*VP8DataTunnel{tb})
	pa := &MultiTrackKCPTunnel{mt: ma}
	pb := &MultiTrackKCPTunnel{mt: mb}
	a := newTrackKCPSession(pa, ta, kcpConvBase, 256)
	b := newTrackKCPSession(pb, tb, kcpConvBase, 256)
	data := tunnel.EncodeFrame(1, tunnel.MsgData, bytes.Repeat([]byte{42}, KCPStreamReadSize))
	a.send(data)
	segments := 0
	var delivered []byte
	for len(ta.sendQueue) > 0 {
		packet := <-ta.sendQueue
		segments++
		for _, message := range b.input(packet[1:]) {
			delivered = append(delivered, message...)
		}
	}
	if segments < 2 || !bytes.Equal(delivered, data) {
		t.Fatal("крупный блок не восстановлен")
	}
	if len(tb.sendQueue) == 0 {
		t.Fatal("подтверждения не отправлены немедленно")
	}
}

func TestОкноKCPОграничено(t *testing.T) {
	for _, pair := range [][2]int{{0, 0}, {24, 30}, {30, 80}, {60, 120}} {
		w := computeKCPWindow(pair[0], pair[1])
		if w < 256 || w > 2048 {
			t.Fatal("окно вышло за безопасные пределы")
		}
	}
}
