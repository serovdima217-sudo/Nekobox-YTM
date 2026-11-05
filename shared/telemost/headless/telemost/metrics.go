// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
	"whitelist-bypass/relay/bond"
)

// Счётчики относятся к relay-блокам, включая повторы, но не к padding RTP/IP.
// Пара — место сервера, не IP и не постоянная личность пользователя.
type splitPairMetrics struct {
	Slot   int          `json:"slot"`
	Leased bool         `json:"leased"`
	Ready  bool         `json:"ready"`
	Up     uint64       `json:"up"`
	Down   uint64       `json:"down"`
	TCP    int          `json:"tcp"`
	UDP    int          `json:"udp"`
	Lanes  []bond.Stats `json:"lanes"`
}
type splitMetrics struct {
	Schema    int                `json:"schema"`
	Protocol  string             `json:"protocol"`
	Instance  string             `json:"instance"`
	PID       int                `json:"pid"`
	Timestamp int64              `json:"timestamp"`
	Running   bool               `json:"running"`
	Capacity  int                `json:"capacity"`
	Leased    int                `json:"leased"`
	Active    int                `json:"active"`
	Pairs     []splitPairMetrics `json:"pairs"`
}

func laneTotals(lanes []bond.Stats) (up, down uint64) {
	for _, lane := range lanes {
		up += lane.Received
		down += lane.Sent
	}
	return
}
func (p *bondPool) metrics(instance string, running bool) splitMetrics {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := splitMetrics{Schema: 1, Protocol: "SplitYTM", Instance: instance, PID: os.Getpid(), Timestamp: time.Now().Unix(), Running: running && !p.closed, Capacity: len(p.groups), Pairs: make([]splitPairMetrics, 0, len(p.groups))}
	for i, g := range p.groups {
		lanes := g.transport.Stats()
		up, down := laneTotals(lanes)
		item := splitPairMetrics{Slot: i + 1, Up: g.previousUp + up, Down: g.previousDown + down, Lanes: lanes}
		if p.leases != nil {
			_, item.Leased = p.leases.Owner(i)
		}
		verified := false
		for _, l := range lanes {
			verified = verified || l.Verified
		}
		select {
		case <-g.transport.Ready():
			item.Ready = item.Leased && verified && g.transport.Err() == nil && m.Running
		default:
		}
		if item.Leased {
			m.Leased++
		}
		if item.Ready {
			m.Active++
		}
		if g.bridge != nil {
			item.TCP, item.UDP, _ = g.bridge.Stats()
		}
		m.Pairs = append(m.Pairs, item)
	}
	return m
}
func writeSplitMetrics(path string, m splitMetrics) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".splitytm-stats-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func startSplitMetrics(ctx context.Context, p *bondPool, path string) func() {
	if p == nil || path == "" {
		return func() {}
	}
	child, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	instance := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	go func() {
		defer close(done)
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		save := func(running bool) {
			if err := writeSplitMetrics(path, p.metrics(instance, running)); err != nil {
				log.Printf("Не удалось сохранить статистику SplitYTM: %v", err)
			}
		}
		save(true)
		for {
			select {
			case <-child.Done():
				save(false)
				return
			case <-tick.C:
				save(true)
			}
		}
	}()
	return func() { cancel(); <-done }
}
