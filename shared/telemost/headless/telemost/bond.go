// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"fmt"
	"log"
	"sync"

	"time"
	"whitelist-bypass/internal/roomlease"
	"whitelist-bypass/relay/bond"
	"whitelist-bypass/relay/tunnel"
	"whitelist-bypass/relay/tunnel/rtc"
)

type bondedGroup struct {
	previousUp, previousDown uint64
	key                      [32]byte
	transport                *bond.Transport
	bridge                   *tunnel.RelayBridge
	lanes                    [bond.Lanes]tunnel.DataTunnel
}
type bondPool struct {
	width     int
	mu        sync.Mutex
	groups    []*bondedGroup
	closed    bool
	paired    bool
	leases    *roomlease.Book
	stop      chan struct{}
	selective bool
	profile   bond.SelectiveProfile
}

func newBondPool(links []string, widths ...int) (*bondPool, error) {
	return newPool(links, false, widths...)
}
func newPool(links []string, paired bool, widths ...int) (*bondPool, error) {
	width := bond.Lanes
	if len(widths) > 0 {
		width = widths[0]
	}

	if width < 1 || width > bond.Lanes || len(links) == 0 || len(links)%width != 0 {
		return nil, fmt.Errorf("число ссылок должно быть кратно размеру группы")
	}
	p := &bondPool{width: width, stop: make(chan struct{}), paired: paired, selective: paired && width == 2, profile: bond.DefaultSelectiveProfile()}
	if paired {
		p.leases, _ = roomlease.New(len(links)/width, 75*time.Second)
	}
	for i := 0; i < len(links); i += width {
		key, e := bond.GroupKey(links[i : i+width])
		if e != nil {
			return nil, e
		}
		g := &bondedGroup{key: key}
		p.groups = append(p.groups, g)
	}
	for i := range p.groups {
		p.restartLocked(i)
	}
	if paired {
		go p.expireLeases()
	}
	return p, nil
}
func (p *bondPool) restartLocked(index int) {
	g := p.groups[index]
	// Сохраняем накопленные байты пары при смене эпохи или клиента.
	if g.transport != nil {
		up, down := laneTotals(g.transport.Stats())
		g.previousUp += up
		g.previousDown += down
	}
	logger := func(format string, args ...any) {
		log.Printf(fmt.Sprintf("[группа %d] ", index+1)+format, args...)
	}
	var t *bond.Transport
	if p.paired {
		if p.selective {
			t = bond.NewSelective(g.key, true, logger)
		} else {
			t = bond.NewPaired(g.key, true, logger, 80*time.Millisecond)
		}
		t.SetLeaseCallbacks(func(id [16]byte) { p.leases.Touch(index, roomlease.Owner(id)) }, func(id [16]byte) { p.leases.Release(index, roomlease.Owner(id)) })
		if owner, ok := p.leases.Owner(index); ok {
			t.BindClient([16]byte(owner))
		}
	} else {
		t = bond.New(g.key, true, logger, p.width)
	}
	rb := tunnel.NewRelayBridge(t, "creator", rtc.KCPStreamReadSize, log.Printf)
	g.transport = t
	g.bridge = rb
	t.SetOnClose(func() { rb.Close(); go p.restart(index, t) })
	t.SetOnReady(func() { log.Printf("[группа %d] все %d каналов проверены", index+1, p.width) })
	for i, raw := range g.lanes[:p.width] {
		if raw != nil {
			t.Attach(i, raw)
		}
	}
}
func (p *bondPool) restart(index int, old *bond.Transport) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.groups[index].transport != old {
		return
	}
	p.restartLocked(index)
}
func (p *bondPool) current(index int) *bond.Transport {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	return p.groups[index].transport
}
func (p *bondPool) Attach(slot int, raw tunnel.DataTunnel, config func(int, int, int)) {
	index, lane := (slot-1)/p.width, (slot-1)%p.width
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	g := p.groups[index]
	if g.lanes[lane] != nil && g.lanes[lane] != raw && !p.paired {
		g.lanes[lane] = raw
		g.transport.Close()
		p.restartLocked(index)
	} else {
		g.lanes[lane] = raw
		g.transport.Attach(lane, raw)
	}
	p.mu.Unlock()
	raw.SetOnData(func(data []byte) {
		if tunnel.LooksLikeRelayFrame(data) {
			tunnel.DecodeFrames(data, func(id uint32, kind byte, payload []byte) {
				if id == bond.LaneConnID && kind == tunnel.MsgData {
					if t := p.current(index); t != nil {
						t.Handle(lane, raw, payload)
					}
					return
				}
				if id != tunnel.ControlConnID || kind != tunnel.MsgConfig {
					return
				}
				fps, batch, tracks, ok := tunnel.DecodeVP8Config(payload)
				if !ok || fps < 1 || fps > 60 || batch < 1 || batch > 120 || tracks < 1 || tracks > 2 {
					return
				}
				if p.paired {
					fps, batch, tracks = 12, 40, 1
					if p.selective {
						fps, batch = p.profile.Carrier(lane)
					}
				}
				raw.Reconfigure(fps, batch)
				raw.SendData(tunnel.EncodeFrame(tunnel.ControlConnID, tunnel.MsgConfigAck, nil))
				if config != nil {
					config(fps, batch, tracks)
				}
			})
			return
		}
		if t := p.current(index); t != nil {
			t.Handle(lane, raw, data)
		}
	})
	raw.SetOnClose(func() { p.Detach(slot, raw) })
}
func (p *bondPool) Detach(slot int, raw tunnel.DataTunnel) {
	index, lane := (slot-1)/p.width, (slot-1)%p.width
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	g := p.groups[index]
	if g.lanes[lane] == raw {
		g.lanes[lane] = nil
		g.transport.Detach(lane, raw)
	}
}
func (p *bondPool) Invalidate(slot int) {
	index, lane := (slot-1)/p.width, (slot-1)%p.width
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	g := p.groups[index]
	raw := g.lanes[lane]
	g.lanes[lane] = nil
	if p.paired {
		if raw != nil {
			g.transport.Detach(lane, raw)
		}
		return
	}
	g.transport.Close()
	p.restartLocked(index)
}
func (p *bondPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	close(p.stop)
	for _, g := range p.groups {
		g.bridge.Close()
		g.transport.Close()
	}
}
