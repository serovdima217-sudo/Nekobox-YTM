// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"whitelist-bypass/internal/roomlease"
	"whitelist-bypass/relay/bond"
	"whitelist-bypass/relay/tunnel"
)

func TestАтомарнаяБроньДвадцатиКомнатПятьюКлиентами(t *testing.T) {
	p, e := newPairedPool(testLinks(20))
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	var wg sync.WaitGroup
	var winners atomic.Int32
	for i := 1; i <= 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := roomlease.Name(p.groups[0].key, roomlease.Owner{byte(i)})
			if p.Admit(1, name) {
				winners.Add(1)
				for slot := 2; slot <= 4; slot++ {
					if !p.Admit(slot, name) {
						t.Error("не зарезервирована вся четвёрка")
					}
				}
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatal("четвёрка досталась разным клиентам")
	}
	for g := 1; g < 5; g++ {
		name := roomlease.Name(p.groups[g].key, roomlease.Owner{byte(100 + g)})
		for lane := 0; lane < 4; lane++ {
			if !p.Admit(g*4+lane+1, name) {
				t.Fatal("не занята независимая четвёрка")
			}
		}
	}
}
func TestНельзяЗанятьЧастьЧужогоНабора(t *testing.T) {
	p, _ := newPairedPool(testLinks(20))
	defer p.Close()
	a, b := roomlease.Name(p.groups[0].key, roomlease.Owner{1}), roomlease.Name(p.groups[0].key, roomlease.Owner{2})
	if p.Admit(2, a) {
		t.Fatal("вторичная комната выдала частичную бронь")
	}
	if !p.Admit(1, a) {
		t.Fatal("первый владелец не допущен")
	}
	for slot := 1; slot <= 4; slot++ {
		if p.Admit(slot, b) || p.OwnsName(slot, b) {
			t.Fatal("чужой клиент захватил комнату")
		}
	}
	if p.Admit(5, a) {
		t.Fatal("подпись первой группы принята во второй")
	}
}
func TestСбойОднойСессииНеПересоздаётГруппу(t *testing.T) {
	p, _ := newPairedPool(testLinks(4))
	defer p.Close()
	raw := &configLane{}
	p.Attach(1, raw, nil)
	old := p.current(0)
	p.Invalidate(1)
	if p.current(0) != old {
		t.Fatal("потеря комнаты пересоздала поток")
	}
	select {
	case <-old.Done():
		t.Fatal("транспорт закрыт")
	default:
	}
	p.Attach(1, &configLane{}, nil)
	if p.current(0) != old {
		t.Fatal("новый медиапуть пересоздал поток")
	}
}
func TestПрофильЧетырёхСессийНеВключаетДемонстрацию(t *testing.T) {
	p, _ := newPairedPool(testLinks(4))
	defer p.Close()
	raw := &configLane{}
	tracks := 0
	p.Attach(1, raw, func(_, _, n int) { tracks = n })
	raw.mu.Lock()
	fn := raw.onData
	raw.mu.Unlock()
	fn(tunnel.EncodeVP8Config(60, 120, 2))
	raw.mu.Lock()
	fps, batch, acks := raw.fps, raw.batch, raw.acks
	raw.mu.Unlock()
	if fps != 12 || batch != 40 || tracks != 1 || acks != 1 {
		t.Fatal("не применён половинный камерный профиль")
	}
	relay := &SFURelay{cameraOnly: true}
	if relay.AddSharingDataChannel() == nil {
		t.Fatal("демонстрация разрешена")
	}
}

func TestОткрытаяКомнатаРезервируетЧетвёркуПриВходе(t *testing.T) {
	p, _ := newPairedPool(testLinks(4))
	defer p.Close()
	name := roomlease.Name(p.groups[0].key, roomlease.Owner{1})
	b := &Bridge{managed: true, bondPool: p, slot: 1, connInfo: &ConnInfo{PeerID: "создатель"}, peers: make(map[string]string), seenPids: make(map[string]bool)}
	b.applyDescriptionEntry(map[string]interface{}{"id": "клиент", "meta": map[string]interface{}{"name": name}})
	if !p.OwnsName(1, name) || b.clientPeer != "клиент" {
		t.Fatal("открытая комната не заняла четвёрку")
	}
	for i := 2; i <= 4; i++ {
		if !p.Admit(i, name) {
			t.Fatal("остальные сессии не забронированы")
		}
	}
}

func TestУстаревшийИдентификаторУчастникаНеБлокируетВладельца(t *testing.T) {
	p, _ := newPairedPool(testLinks(4))
	defer p.Close()
	name := roomlease.Name(p.groups[0].key, roomlease.Owner{1})
	if !p.Admit(1, name) {
		t.Fatal("нет брони")
	}
	b := &Bridge{managed: true, bondPool: p, slot: 1, clientPeer: "старый участник", connInfo: &ConnInfo{PeerID: "создатель"}, peers: make(map[string]string), seenPids: make(map[string]bool)}
	b.applyDescriptionEntry(map[string]interface{}{"id": "новый участник", "meta": map[string]interface{}{"name": name}})
	if b.clientPeer != "новый участник" {
		t.Fatal("устаревшая запись не освобождена")
	}
	b.applyDescriptionEntry(map[string]interface{}{"id": "новый участник"})
	if !p.OwnsName(1, b.peers["новый участник"]) {
		t.Fatal("неполное обновление потеряло имя владельца")
	}
}

func TestПятьКлиентовБронируютДесятьКомнатБезПересечений(t *testing.T) {
	p, e := newSelectivePool(testLinks(10), bond.DefaultSelectiveProfile())
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	for g := 0; g < 5; g++ {
		name := roomlease.NameForWidth(p.groups[g].key, roomlease.Owner{byte(g + 1)}, 2)
		if !p.Admit(g*2+1, name) || !p.Admit(g*2+2, name) {
			t.Fatal("не занята целая пара")
		}
		if p.Admit(g*2+1, roomlease.NameForWidth(p.groups[g].key, roomlease.Owner{100}, 2)) {
			t.Fatal("допущен чужой владелец")
		}
	}
	raw := &configLane{}
	p.Attach(2, raw, nil)
	raw.mu.Lock()
	fn := raw.onData
	raw.mu.Unlock()
	fn(tunnel.EncodeVP8Config(60, 120, 2))
	raw.mu.Lock()
	defer raw.mu.Unlock()
	if raw.fps != 12 || raw.batch != 40 {
		t.Fatal("резерву назначен основной профиль")
	}
}
