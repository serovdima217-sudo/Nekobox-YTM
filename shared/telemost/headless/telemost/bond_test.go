// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"whitelist-bypass/relay/tunnel"
)

type configLane struct {
	mu         sync.Mutex
	onData     func([]byte)
	onClose    func()
	fps, batch int
	acks       int
}

func (f *configLane) SendData(p []byte) {
	tunnel.DecodeFrames(p, func(id uint32, kind byte, _ []byte) {
		if id == 0 && kind == tunnel.MsgConfigAck {
			f.mu.Lock()
			f.acks++
			f.mu.Unlock()
		}
	})
}
func (f *configLane) SetOnData(fn func([]byte)) { f.mu.Lock(); f.onData = fn; f.mu.Unlock() }
func (f *configLane) SetOnClose(fn func())      { f.mu.Lock(); f.onClose = fn; f.mu.Unlock() }
func (f *configLane) Reconfigure(fps, batch int) {
	f.mu.Lock()
	f.fps = fps
	f.batch = batch
	f.mu.Unlock()
}
func testLinks(n int) []string {
	r := make([]string, n)
	for i := range r {
		r[i] = fmt.Sprintf("https://telemost.yandex.ru/j/%d", 700000+i)
	}
	return r
}
func TestПятьГруппИзПятидесятиКомнат(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]any{"sessions": 50, "auto_create": false, "links": testLinks(50), "bond_size": 10, "join_interval_ms": 250}
	data, _ := json.Marshal(cfg)
	path := filepath.Join(dir, "server.json")
	os.WriteFile(path, data, 0600)
	c, e := readManagedConfig(path)
	if e != nil {
		t.Fatal(e)
	}
	p, e := newBondPool(c.Links)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	if len(p.groups) != 5 {
		t.Fatal("не созданы пять групп")
	}
	first, other := p.current(0), p.current(1)
	p.Invalidate(1)
	select {
	case <-first.Done():
	default:
		t.Fatal("старая группа не закрыта")
	}
	if p.current(0) == first || p.current(1) != other {
		t.Fatal("перезапуск нарушил изоляцию групп")
	}
}
func TestНастройкаКамерыИДемонстрацииНеТеряетсяВСлиянии(t *testing.T) {
	p, e := newBondPool(testLinks(10))
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	raw := &configLane{}
	tracks := 0
	p.Attach(1, raw, func(_, _, n int) { tracks = n })
	raw.mu.Lock()
	fn := raw.onData
	raw.mu.Unlock()
	fn(tunnel.EncodeVP8Config(24, 40, 2))
	raw.mu.Lock()
	fps, batch, acks := raw.fps, raw.batch, raw.acks
	raw.mu.Unlock()
	if fps != 24 || batch != 40 || acks != 1 || tracks != 2 {
		t.Fatal("служебная настройка не прошла")
	}
	fn(tunnel.EncodeVP8Config(600, 400, 2))
	raw.mu.Lock()
	defer raw.mu.Unlock()
	if raw.acks != 1 {
		t.Fatal("опасная частота принята")
	}
}
func TestНеполныеГруппыВКонфигурацииОтклоняются(t *testing.T) {
	for _, cfg := range []map[string]any{
		{"sessions": 10, "auto_create": true, "links": testLinks(10), "bond_size": 10},
		{"sessions": 11, "auto_create": false, "links": testLinks(11), "bond_size": 10},
		{"sessions": 10, "auto_create": false, "links": testLinks(9), "bond_size": 10},
		{"sessions": 10, "auto_create": false, "links": testLinks(10), "bond_size": 11},
	} {
		b, _ := json.Marshal(cfg)
		path := filepath.Join(t.TempDir(), "server.json")
		os.WriteFile(path, b, 0600)
		if _, e := readManagedConfig(path); e == nil {
			t.Fatal("принята неполная или неустойчивая группа")
		}
	}
}

func TestГруппировкаРазногоЧислаКомнат(t *testing.T) {
	for _, width := range []int{1, 2, 3, 5, 8, 10} {
		p, e := newBondPool(testLinks(width*5), width)
		if e != nil {
			t.Fatal(e)
		}
		if len(p.groups) != 5 || len(p.current(0).Stats()) != width {
			t.Fatal("неверное число групп или каналов")
		}
		raw := &configLane{}
		p.Attach(width+1, raw, nil)
		p.mu.Lock()
		matched := p.groups[1].lanes[0] == raw && p.groups[0].lanes[0] == nil
		p.mu.Unlock()
		p.Close()
		if !matched {
			t.Fatal("канал попал не в ту группу")
		}
	}
}
