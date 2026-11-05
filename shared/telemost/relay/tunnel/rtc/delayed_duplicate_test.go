// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package rtc

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"whitelist-bypass/relay/tunnel"
)

// Проверка общей KCP-сессии: камера полностью потеряна, все данные доставляет демка.
func TestKCPВосстановлениеЧерезДемку(t *testing.T) {
	log := func(string, ...any) {}
	camA, screenA := NewVP8DataTunnel(nil, nil, log), NewVP8DataTunnel(nil, nil, log)
	camB := NewVP8DataTunnel(nil, nil, log)
	a, b := NewMultiTrackTunnel([]*VP8DataTunnel{camA, screenA}), NewMultiTrackTunnel([]*VP8DataTunnel{camB})
	a.EnableDelayedDuplicate(25 * time.Millisecond)
	b.EnableDuplicateReceiver()
	ka, kb := NewMultiTrackKCPTunnel(a, log), NewMultiTrackKCPTunnel(b, log)
	defer ka.Stop()
	defer kb.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	pump := func(ch <-chan []byte, dst *MultiTrackTunnel, drop bool, screen bool) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case p := <-ch:
					if !drop {
						dst.deliver(p, screen)
					}
				}
			}
		}()
	}
	pump(camA.sendQueue, b, true, false)
	pump(screenA.sendQueue, b, false, true)
	pump(camB.sendQueue, a, false, false)
	defer func() { cancel(); wg.Wait() }()
	got := make(chan []byte, 64)
	kb.SetOnData(func(p []byte) { got <- p })
	expected := make([][]byte, 40)
	for i := range expected {
		expected[i] = tunnel.EncodeFrame(7, tunnel.MsgData, bytes.Repeat([]byte(fmt.Sprintf("данные-%02d", i)), 30))
		ka.SendData(expected[i])
	}
	for i, p := range expected {
		select {
		case actual := <-got:
			if !bytes.Equal(actual, p) {
				t.Fatalf("неверный блок %d", i)
			}
		case <-ctx.Done():
			t.Fatal("данные через демонстрацию не восстановлены")
		}
	}
	select {
	case <-got:
		t.Fatal("повтор блока попал в приложение")
	case <-time.After(100 * time.Millisecond):
	}
	if ka.mt.duplicate.Stats().Secondary == 0 {
		t.Fatal("демка не отправляла данные")
	}
	if len(ka.sessions) != 1 {
		t.Fatal("вместо одной KCP-сессии созданы независимые")
	}
}

// Пять независимых транспортов одновременно, каждый со своей общей KCP-сессией.
func TestПятьОдновременныхСессийСДемкой(t *testing.T) {
	for i := 0; i < 5; i++ {
		t.Run(fmt.Sprintf("сессия-%d", i+1), func(t *testing.T) { t.Parallel(); TestKCPВосстановлениеЧерезДемку(t) })
	}
}
