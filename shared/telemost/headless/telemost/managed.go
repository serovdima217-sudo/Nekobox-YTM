// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/kulikov0/headless-client"
	"github.com/kulikov0/headless-client/websocket"
	"whitelist-bypass/internal/roommgr"
	"whitelist-bypass/relay/bond"
	"whitelist-bypass/relay/common"
)

type managedConfig struct {
	Clients           int      `json:"clients"`
	SelectiveSessions bool     `json:"selective_sessions"`
	PrimaryFPS        int      `json:"primary_fps"`
	PrimaryBatch      int      `json:"primary_batch"`
	ReserveFPS        int      `json:"reserve_fps"`
	ReserveBatch      int      `json:"reserve_batch"`
	PairedSessions    bool     `json:"paired_sessions"`
	BondSize          int      `json:"bond_size"`
	JoinIntervalMS    int      `json:"join_interval_ms"`
	Sessions          int      `json:"sessions"`
	AutoCreate        bool     `json:"auto_create"`
	Links             []string `json:"links"`
	CookieFile        string   `json:"cookie_file"`
	StateFile         string   `json:"state_file"`
	LinksFile         string   `json:"links_file"`
	MemoryMiB         int64    `json:"memory_limit_mib"`
	DuplicateMS       int      `json:"duplicate_delay_ms"`
	CreateSeconds     int      `json:"create_interval_seconds"`
	RetrySeconds      int      `json:"retry_interval_seconds"`
}

func readManagedConfig(path string) (managedConfig, error) {
	c := managedConfig{PrimaryFPS: 60, PrimaryBatch: 120, ReserveFPS: 12, ReserveBatch: 40, Sessions: 5, AutoCreate: true, CookieFile: "yandex-cookie.txt", StateFile: "telemost-rooms.json", LinksFile: "links.txt", MemoryMiB: 150, DuplicateMS: 80, CreateSeconds: 3, RetrySeconds: 15}
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, fmt.Errorf("не удалось прочитать JSON-конфигурацию")
	}
	if d.Decode(new(any)) != io.EOF {
		return c, fmt.Errorf("лишние данные после конфигурации")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(b, &fields) != nil || fields == nil {
		return c, fmt.Errorf("конфигурация должна быть JSON-объектом")
	}
	if _, hasClients := fields["clients"]; hasClients {
		if c.Clients < 1 || c.Clients > 25 || len(c.Links) < 2*c.Clients {
			return c, fmt.Errorf("для каждого из 1–25 клиентов нужны две разные готовые ссылки")
		}
		c.Sessions = 2 * c.Clients
		c.Links = c.Links[:c.Sessions]
		c.BondSize = 2
		c.SelectiveSessions = true
		c.PairedSessions = false
		c.AutoCreate = false
	}
	if c.Sessions < 1 || c.Sessions > roommgr.MaxRooms || len(c.Links) > c.Sessions {
		return c, fmt.Errorf("комнат должно быть от 1 до 50, ссылок не больше числа комнат")
	}
	if c.BondSize != 0 && (c.BondSize < 1 || c.BondSize > 10 || c.Sessions%c.BondSize != 0 || c.AutoCreate || len(c.Links) != c.Sessions) {
		return c, fmt.Errorf("объединение требует bond_size от 1 до 10, готовые ссылки, auto_create=false и число комнат кратное размеру группы")
	}
	if c.SelectiveSessions && (c.PairedSessions || c.BondSize != 2 || !c.selectiveProfile().Valid()) {
		return c, fmt.Errorf("режим основной+резерв требует пары комнат и корректные разные профили")
	}
	if c.PairedSessions && (c.BondSize != 4 || c.Sessions%4 != 0) {
		return c, fmt.Errorf("режим 2+2 требует группы ровно из четырёх готовых комнат")
	}
	if c.PairedSessions && c.DuplicateMS != 80 {
		return c, fmt.Errorf("в режиме 2+2 задержка копий фиксирована: 80 мс")
	}
	if c.JoinIntervalMS < 0 || c.JoinIntervalMS > 3000 {
		return c, fmt.Errorf("интервал подключения должен быть 0–3000 мс")
	}

	if c.DuplicateMS < 0 || c.DuplicateMS > 2000 || c.MemoryMiB < 32 || c.MemoryMiB > 1024 {
		return c, fmt.Errorf("неверный лимит памяти или задержка копий")
	}
	if c.CreateSeconds < 1 || c.CreateSeconds > 300 || c.RetrySeconds < 1 || c.RetrySeconds > 300 {
		return c, fmt.Errorf("интервалы должны быть от 1 до 300 секунд")
	}
	base, e := filepath.Abs(filepath.Dir(path))
	if e != nil {
		return c, e
	}
	for _, p := range []*string{&c.CookieFile, &c.StateFile, &c.LinksFile} {
		if *p == "" {
			return c, fmt.Errorf("не задан путь к файлу")
		}
		if !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
		*p = filepath.Clean(*p)
	}
	configAbs, _ := filepath.Abs(path)
	if c.CookieFile == c.StateFile || c.CookieFile == c.LinksFile || c.StateFile == c.LinksFile || c.StateFile == configAbs || c.LinksFile == configAbs {
		return c, fmt.Errorf("пути cookie, состояния, ссылок и конфигурации должны различаться")
	}
	return c, nil
}
func (c managedConfig) options() roommgr.Options {
	return roommgr.Options{Sessions: c.Sessions, AutoCreate: c.AutoCreate, Links: c.Links, StateFile: c.StateFile, LinksFile: c.LinksFile, RetryDelay: time.Duration(c.RetrySeconds) * time.Second, CreateDelay: time.Duration(c.CreateSeconds) * time.Second, OnStatus: func(slot int, status string) { log.Printf("[место %d] %s", slot, status) }}
}

type telemostBackend struct {
	cookie       string
	cfg          TMConfig
	duplicate    time.Duration
	bonds        *bondPool
	joinInterval time.Duration
}

func (b *telemostBackend) Create(ctx context.Context, slot int) (string, error) {
	cfg := b.cfg
	cfg.Context = ctx
	cfg.InstanceID = uuid.NewString()
	raw, status, e := tmRequest("POST", "/conferences?next_gen_media_platform_allowed=true", struct{}{}, b.cookie, cfg)
	if e != nil {
		return "", fmt.Errorf("сетевая ошибка создания комнаты")
	}
	if status != 200 && status != 201 {
		return "", fmt.Errorf("создание комнаты отклонено: HTTP %d", status)
	}
	var v struct {
		URI string `json:"uri"`
	}
	if json.Unmarshal(raw, &v) != nil || v.URI == "" {
		return "", fmt.Errorf("в ответе нет ссылки")
	}
	return v.URI, nil
}
func (b *telemostBackend) Run(ctx context.Context, slot int, link string) error {
	if b.joinInterval > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(slot-1) * b.joinInterval):
		}
	}

	cfg := b.cfg
	cfg.Context = ctx
	cfg.InstanceID = uuid.NewString()
	info, e := joinExistingConference(b.cookie, link, cfg)
	if e != nil {
		return e
	}
	bridge := &Bridge{bondPool: b.bonds, slot: slot, ctx: ctx, managed: true, duplicateDelay: b.duplicate, connInfo: info, config: cfg, cookieStr: b.cookie, peers: make(map[string]string), seenPids: make(map[string]bool), readBuf: 16384}
	return bridge.runOnce(ctx)
}

func (b *Bridge) runOnce(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	b.ctx = ctx
	b.config.Context = ctx
	dialer := headless.ChromeWindows.WebSocketDialer(headless.TLSOptions{})
	dialer.HandshakeTimeout = 20 * time.Second
	header := headless.ChromeWindows.Headers(headless.DestWebSocket)
	header.Set("Origin", tmOrigin)
	ws, _, e := dialer.DialContext(ctx, b.connInfo.MediaServerURL, header)
	if e != nil {
		return fmt.Errorf("не удалось подключить сигнализацию")
	}
	b.mu.Lock()
	b.ws = ws
	b.mu.Unlock()
	stop := context.AfterFunc(ctx, func() { ws.Close() })
	defer stop()
	var workers sync.WaitGroup
	defer func() {
		cancel()
		ws.Close()
		workers.Wait()
		b.mu.Lock()
		b.ws = nil
		b.mu.Unlock()
		if b.bondPool != nil {
			b.bondPool.Invalidate(b.slot)
		}
		if b.relay != nil {
			b.relay.Close()
		}
		b.connMu.Lock()
		bridge := b.activeBridge
		b.activeBridge = nil
		b.connMu.Unlock()
		if bridge != nil {
			bridge.Close()
		}
	}()
	periodic := func(interval time.Duration, immediate bool, fn func()) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if immediate {
				fn()
			}
			t := time.NewTicker(interval)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					fn()
				}
			}
		}()
	}
	b.sendHello()
	periodic(5*time.Second, true, b.pollAndAdmit)
	periodic(tmPingPeriod, false, func() { b.wsSend(map[string]interface{}{"uid": uuid.NewString(), "ping": map[string]interface{}{}}) })
	interval := b.connInfo.StateCheckIntervalS
	if interval < 1 {
		interval = 30
	}
	periodic(time.Duration(interval)*time.Second, true, func() { b.requestStates() })
	for ctx.Err() == nil {
		_, raw, e := ws.ReadMessage()
		if e != nil {
			if ce, ok := e.(*websocket.CloseError); ok {
				log.Printf("[место %d] сигнализация закрылась с кодом %d", b.slot, ce.Code)
			} else {
				log.Printf("[место %d] сигнализация прервалась: %T", b.slot, e)
			}
			return fmt.Errorf("сигнализация отключилась")
		}
		b.handleMessage(raw)
	}
	return ctx.Err()
}

func runManaged(path string, check bool) error {
	c, e := readManagedConfig(path)
	if e != nil {
		return e
	}
	if _, e = roommgr.New(c.options(), nil); e != nil {
		return e
	}
	if check {
		fmt.Printf("Конфигурация корректна: %d комнат, автосоздание %t, задержка %d мс, мягкий лимит %d МиБ.\n", c.Sessions, c.AutoCreate, c.DuplicateMS, c.MemoryMiB)
		return nil
	}
	if e = os.MkdirAll(filepath.Dir(c.StateFile), 0700); e != nil {
		return e
	}
	release, e := lockManaged(c.StateFile + ".lock")
	if e != nil {
		return e
	}
	defer release()
	cookie := strings.TrimSpace(os.Getenv("YANDEX_COOKIE"))
	if cookie == "" {
		raw, e := os.ReadFile(c.CookieFile)
		if e != nil {
			return fmt.Errorf("не удалось прочитать файл cookie Яндекса")
		}
		cookie = strings.TrimSpace(string(raw))
	}
	if cookie == "" {
		return fmt.Errorf("cookie Яндекса пуст")
	}
	if !strings.Contains(cookie, "=") {
		cookie = "Session_id=" + cookie
	}
	if common.CookieValue(cookie, "Session_id") == "" {
		return fmt.Errorf("в cookie нет Session_id")
	}
	debug.SetMemoryLimit(c.MemoryMiB << 20)
	debug.SetGCPercent(50)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	cfg := fetchConfig()
	// Один HTTP/2-пул обслуживает все комнаты; отдельный TLS-пул на каждый опрос накапливает память.
	cfg.HTTP = &http.Client{Timeout: 20 * time.Second, Transport: headless.ChromeWindows.Transport(headless.TLSOptions{})}
	defer cfg.HTTP.CloseIdleConnections()
	backend := &telemostBackend{cookie: cookie, cfg: cfg, duplicate: time.Duration(c.DuplicateMS) * time.Millisecond, joinInterval: time.Duration(c.JoinIntervalMS) * time.Millisecond}
	if c.BondSize > 0 {
		if c.SelectiveSessions {
			backend.duplicate = 0
			backend.bonds, e = newSelectivePool(c.Links, c.selectiveProfile())
		} else if c.PairedSessions {
			backend.duplicate = 0
			backend.bonds, e = newPairedPool(c.Links)
		} else {
			backend.bonds, e = newBondPool(c.Links, c.BondSize)
		}
		if e != nil {
			return e
		}
		defer backend.bonds.Close()
		stopBonds := context.AfterFunc(ctx, backend.bonds.Close)
		defer stopBonds()
	}
	stopMetrics := startSplitMetrics(ctx, backend.bonds, os.Getenv("TM_SPLIT_STATS_FILE"))
	defer stopMetrics()
	manager, e := roommgr.New(c.options(), backend)
	if e != nil {
		return e
	}
	log.Printf("Один процесс управляет %d независимыми комнатами. Ссылки: %s", c.Sessions, c.LinksFile)
	return manager.Run(ctx)
}

// Проверка используется тестами: структура экземпляра API не общая между комнатами.
var _ roommgr.Backend = (*telemostBackend)(nil)

func (c managedConfig) selectiveProfile() bond.SelectiveProfile {
	return bond.SelectiveProfile{MainFPS: c.PrimaryFPS, MainBatch: c.PrimaryBatch, ReserveFPS: c.ReserveFPS, ReserveBatch: c.ReserveBatch, Mode: bond.Balanced}
}
