// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package rtc

import (
	"encoding/binary"
	"sync"
	"time"
	"whitelist-bypass/relay/dup"
)

type MultiTrackTunnel struct {
	duplicate     *dup.Carrier
	duplicateSend bool
	tunnels       []*VP8DataTunnel

	mu            sync.Mutex
	onData        func([]byte)
	onClose       func()
	onPeerRestart func()
	isClosed      bool
	fps           int
	batch         int
}

func NewMultiTrackTunnel(tunnels []*VP8DataTunnel) *MultiTrackTunnel {
	m := &MultiTrackTunnel{tunnels: tunnels}
	for i, tun := range tunnels {
		// Only the cam (index 0) carries the cascade-on-close semantics;
		// screenshare tracks close independently during partial shrink.
		m.wireSubTunnel(tun, i == 0)
	}
	return m
}

func (m *MultiTrackTunnel) wireSubTunnel(tun *VP8DataTunnel, isCamera bool) {
	tun.SetOnData(func(data []byte) { m.deliver(data, !isCamera) })

	if !isCamera {
		// Screenshare close is a partial shrink; it must not cascade-Stop
		// the cam writer or notify the parent that the whole tunnel died.
		return
	}
	tun.SetOnPeerRestart(func() {
		m.mu.Lock()
		handler := m.onPeerRestart
		m.mu.Unlock()
		if handler != nil {
			handler()
		}
	})
	tun.SetOnClose(func() {
		m.mu.Lock()
		if m.isClosed {
			m.mu.Unlock()
			return
		}
		m.isClosed = true
		closeHandler := m.onClose
		subTunnels := append([]*VP8DataTunnel(nil), m.tunnels...)
		duplicate := m.duplicate
		m.mu.Unlock()

		if duplicate != nil {
			duplicate.Close()
		}
		for _, t := range subTunnels {
			t.Stop()
		}
		if closeHandler != nil {
			closeHandler()
		}
	})
}

func (m *MultiTrackTunnel) AddSubTunnel(tun *VP8DataTunnel) {
	m.mu.Lock()
	if m.isClosed {
		m.mu.Unlock()
		tun.Stop()
		return
	}
	m.tunnels = append(m.tunnels, tun)
	fps := m.fps
	batch := m.batch
	m.mu.Unlock()
	m.wireSubTunnel(tun, false)
	if fps > 0 && batch > 0 {
		tun.Start(fps, batch)
	}
}

func (m *MultiTrackTunnel) RemoveLastSubTunnel() *VP8DataTunnel {
	m.mu.Lock()
	if len(m.tunnels) <= 1 {
		m.mu.Unlock()
		return nil
	}
	last := m.tunnels[len(m.tunnels)-1]
	m.tunnels = m.tunnels[:len(m.tunnels)-1]
	m.mu.Unlock()
	last.Stop()
	return last
}

func (m *MultiTrackTunnel) SubTunnelCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tunnels)
}

func (m *MultiTrackTunnel) SendData(data []byte) {
	m.mu.Lock()
	d := m.duplicate
	m.mu.Unlock()
	if d != nil && m.DuplicateEnabled() {
		d.Send(data)
		return
	}
	m.mu.Lock()
	tunnels := append([]*VP8DataTunnel(nil), m.tunnels...)
	m.mu.Unlock()
	if len(tunnels) == 0 {
		return
	}
	var connID uint32
	if len(data) >= 8 {
		connID = binary.BigEndian.Uint32(data[4:8])
	}
	idx := connID % uint32(len(tunnels))
	tunnels[idx].SendData(data)
}

func (m *MultiTrackTunnel) DeliverData(data []byte) { m.deliverPlain(data) }
func (m *MultiTrackTunnel) deliverPlain(data []byte) {
	m.mu.Lock()
	handler := m.onData
	m.mu.Unlock()
	if handler != nil {
		handler(data)
	}
}
func (m *MultiTrackTunnel) deliver(data []byte, secondary bool) {
	m.mu.Lock()
	d := m.duplicate
	m.mu.Unlock()
	if d != nil && dup.HasEnvelope(data) {
		payload, ok := d.Receive(data, secondary)
		if !ok {
			return
		}
		data = payload
	}
	m.deliverPlain(data)
}

// EnableDelayedDuplicate включается до передачи пользовательских данных.
func (m *MultiTrackTunnel) EnableDelayedDuplicate(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.duplicate != nil || m.isClosed {
		return
	}
	send := func(index int) func([]byte) bool {
		return func(b []byte) bool {
			m.mu.Lock()
			var t *VP8DataTunnel
			if !m.isClosed && len(m.tunnels) > index {
				t = m.tunnels[index]
			}
			m.mu.Unlock()
			return t != nil && t.TrySendData(b)
		}
	}
	m.duplicateSend = true
	m.duplicate = dup.New(delay, 2048, 2<<20, send(0), send(1))
}
func (m *MultiTrackTunnel) DuplicateEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.duplicate != nil && m.duplicateSend
}
func (m *MultiTrackTunnel) TrySendData(data []byte) bool {
	m.mu.Lock()
	d := m.duplicate
	m.mu.Unlock()
	if d != nil && m.DuplicateEnabled() {
		return d.Send(data)
	}
	m.SendData(data)
	return true
}

func (m *MultiTrackTunnel) SubTunnels() []*VP8DataTunnel {
	m.mu.Lock()
	defer m.mu.Unlock()
	subs := make([]*VP8DataTunnel, len(m.tunnels))
	copy(subs, m.tunnels)
	return subs
}

func (m *MultiTrackTunnel) SetOnData(fn func([]byte)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onData = fn
}

func (m *MultiTrackTunnel) SetOnClose(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onClose = fn
}

func (m *MultiTrackTunnel) SetOnPeerRestart(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPeerRestart = fn
}

func (m *MultiTrackTunnel) Reconfigure(fps, batch int) {
	m.mu.Lock()
	m.fps = fps
	m.batch = batch
	tunnels := append([]*VP8DataTunnel(nil), m.tunnels...)
	m.mu.Unlock()
	for _, tun := range tunnels {
		tun.Reconfigure(fps, batch)
	}
}

func (m *MultiTrackTunnel) Start(fps, batch int) {
	m.mu.Lock()
	m.fps = fps
	m.batch = batch
	tunnels := append([]*VP8DataTunnel(nil), m.tunnels...)
	m.mu.Unlock()
	for _, tun := range tunnels {
		tun.Start(fps, batch)
	}
}

func (m *MultiTrackTunnel) Stop() {
	m.mu.Lock()
	if m.isClosed {
		m.mu.Unlock()
		return
	}
	m.isClosed = true
	duplicate := m.duplicate
	tunnels := append([]*VP8DataTunnel(nil), m.tunnels...)
	m.mu.Unlock()
	if duplicate != nil {
		duplicate.Close()
	}
	for _, tun := range tunnels {
		tun.Stop()
	}
}

func (m *MultiTrackTunnel) HandleFrame(frame []byte) {
	m.mu.Lock()
	var first *VP8DataTunnel
	if len(m.tunnels) > 0 {
		first = m.tunnels[0]
	}
	m.mu.Unlock()
	if first != nil {
		first.HandleFrame(frame)
	}
}

// EnableDuplicateReceiver принимает отложенные копии в направлении сервер → клиент.
// Обратное направление остаётся надёжным камерным каналом KCP, без второй демонстрации.
func (m *MultiTrackTunnel) EnableDuplicateReceiver() {
	m.EnableDelayedDuplicate(time.Millisecond)
	m.mu.Lock()
	m.duplicateSend = false
	m.mu.Unlock()
}
