// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"encoding/json"
	"log"
	"net/url"
	"time"

	"whitelist-bypass/internal/roomlease"
	"whitelist-bypass/relay/bond"
)

func newPairedPool(links []string) (*bondPool, error) { return newPool(links, true, bond.PairedLanes) }

// Admit резервирует четыре ссылки до допуска первого гостя. Остальные комнаты
// допускают только того же владельца; получить половину чужого набора нельзя.
func (p *bondPool) Admit(slot int, name string) bool {
	if !p.paired || slot < 1 || slot > len(p.groups)*p.width {
		return false
	}
	index, lane := (slot-1)/p.width, (slot-1)%p.width
	owner, ok := roomlease.ParseNameForWidth(p.groups[index].key, name, p.width)
	if !ok {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	if lane != 0 {
		current, ok := p.leases.Owner(index)
		return ok && current == owner
	}
	ok, fresh := p.leases.Acquire(index, owner)
	if !ok {
		return false
	}
	if fresh {
		p.groups[index].transport.Close()
		p.restartLocked(index)
		log.Printf("[группа %d] атомарно занят набор из %d ссылок; срок без продления 75 секунд", index+1, p.width)
	}
	return p.groups[index].transport.BindClient([16]byte(owner)) == nil
}
func (p *bondPool) OwnsName(slot int, name string) bool {
	if !p.paired || slot < 1 || slot > len(p.groups)*p.width {
		return false
	}
	index := (slot - 1) / p.width
	owner, ok := roomlease.ParseNameForWidth(p.groups[index].key, name, p.width)
	if !ok {
		return false
	}
	current, ok := p.leases.Owner(index)
	return ok && current == owner
}
func (p *bondPool) expireLeases() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-tick.C:
			p.mu.Lock()
			if p.closed {
				p.mu.Unlock()
				return
			}
			for i, g := range p.groups {
				if _, active := p.leases.Owner(i); !active && g.transport.ClientIdentifier() != ([16]byte{}) {
					g.transport.Close()
					p.restartLocked(i)
					log.Printf("[группа %d] бронь освобождена; старый поток завершён", i+1)
				}
			}
			p.mu.Unlock()
		}
	}
}

// Вызывается с admitMu: сетевой допуск не гоняется с другими опросами комнаты.
func (b *Bridge) pollAndAdmitPaired() {
	b.mu.Lock()
	peers := make(map[string]string, len(b.peers))
	for pid, name := range b.peers {
		peers[pid] = name
	}
	b.mu.Unlock()
	if len(peers) > 0 {
		for pid, name := range peers {
			if !b.bondPool.OwnsName(b.slot, name) {
				b.kickPeer(pid)
			}
		}
		return
	}
	conf := url.QueryEscape(b.connInfo.ConferenceURI)
	raw, status, e := tmRequest("GET", "/conferences/"+conf+"/waiting-rooms/peers", nil, b.cookieStr, b.config)
	if e != nil || status != 200 {
		return
	}
	var resp struct {
		Peers []struct {
			PeerID string `json:"peer_id"`
			State  struct {
				DisplayName string `json:"display_name"`
			} `json:"state"`
		} `json:"peers"`
	}
	if json.Unmarshal(raw, &resp) != nil {
		return
	}
	for _, guest := range resp.Peers {
		if !b.bondPool.Admit(b.slot, guest.State.DisplayName) {
			continue
		}
		_, status, e = tmRequest("PUT", "/conferences/"+conf+"/commands/admit?peer_id="+url.QueryEscape(guest.PeerID), nil, b.cookieStr, b.config)
		if e == nil && status >= 200 && status < 300 {
			log.Printf("[место %d] допущен владелец зарезервированной четвёрки", b.slot)
		}
		return
	}
}

func newSelectivePool(links []string, profile bond.SelectiveProfile) (*bondPool, error) {
	p, e := newPool(links, true, bond.SelectiveLanes)
	if e != nil {
		return nil, e
	}
	p.profile = profile
	return p, nil
}
