// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kulikov0/headless-client/websocket"
)

type fakeHTTP func(*http.Request) (*http.Response, error)

func (f fakeHTTP) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestКонфигурацияМенеджера(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "server.json")
	for _, data := range []string{`{"sessions":0}`, `{"sessions":51}`, `{"state_file":"yandex-cookie.txt"}`, `{"links_file":"server.json"}`, `{"ошибка":true}`, `{} {}`} {
		os.WriteFile(p, []byte(data), 0600)
		if _, e := readManagedConfig(p); e == nil {
			t.Fatalf("неверная конфигурация принята: %s", data)
		}
	}
	os.WriteFile(p, []byte(`{"sessions":5}`), 0600)
	c, e := readManagedConfig(p)
	if e != nil || c.Sessions != 5 || !filepath.IsAbs(c.StateFile) {
		t.Fatal("корректная конфигурация не читается")
	}
	release, e := lockManaged(filepath.Join(dir, "lock"))
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if release2, e := lockManaged(filepath.Join(dir, "lock")); e == nil {
		release2()
		t.Fatal("блокировка не защитила от второго процесса")
	}
}

// Это локальная имитация сигнализации, а не проверка настоящего SFU Телемоста.
func TestПятьСигнализацийИОтмена(t *testing.T) {
	opened := make(chan struct{}, 5)
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := up.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		opened <- struct{}{}
		for {
			if _, _, e = ws.ReadMessage(); e != nil {
				return
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		b := &Bridge{ctx: ctx, managed: true, config: TMConfig{Context: ctx, HTTP: &http.Client{Transport: fakeHTTP(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"peers":[]}`)), Header: make(http.Header), Request: r}, nil
		})}}, connInfo: &ConnInfo{MediaServerURL: "ws" + strings.TrimPrefix(server.URL, "http"), ConferenceURI: "https://telemost.yandex.ru/j/123456"}, peers: make(map[string]string), seenPids: make(map[string]bool)}
		go func() { done <- b.runOnce(ctx) }()
	}
	for i := 0; i < 5; i++ {
		select {
		case <-opened:
		case <-time.After(3 * time.Second):
			t.Fatal("не открылись пять сигнализаций")
		}
	}
	cancel()
	for i := 0; i < 5; i++ {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("отмена не завершила сигнализацию")
		}
	}
}

func TestИзолированныеИдентификаторыAPI(t *testing.T) {
	seen := map[string]bool{}
	c := &http.Client{Transport: fakeHTTP(func(r *http.Request) (*http.Response, error) {
		id := r.Header.Get("Client-Instance-Id")
		if id == "" || seen[id] {
			t.Error("между комнатами повторился идентификатор API")
		}
		seen[id] = true
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header), Request: r}, nil
	})}
	backend := &telemostBackend{cookie: "Session_id=тест", cfg: TMConfig{HTTP: c}}
	for i := 1; i <= 5; i++ {
		if e := backend.Run(context.Background(), i, "https://telemost.yandex.ru/j/123456"); e == nil {
			t.Error("ошибка API проигнорирована")
		}
	}
	if len(seen) != 5 {
		t.Error("не проверены все пять комнат")
	}
}

func TestНетНемедленногоPingДоОтветаНаHello(t *testing.T) {
	result := make(chan bool, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			result <- false
			return
		}
		defer ws.Close()
		var first map[string]interface{}
		if err := ws.ReadJSON(&first); err != nil || first["hello"] == nil {
			result <- false
			return
		}
		ws.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		_, _, err = ws.ReadMessage()
		result <- err != nil
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &Bridge{ctx: ctx, managed: true, config: TMConfig{Context: ctx, HTTP: &http.Client{Transport: fakeHTTP(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"peers":[]}`)), Header: make(http.Header), Request: r}, nil
	})}}, connInfo: &ConnInfo{MediaServerURL: "ws" + strings.TrimPrefix(server.URL, "http"), ConferenceURI: "https://telemost.yandex.ru/j/123456"}, peers: make(map[string]string), seenPids: make(map[string]bool)}
	done := make(chan error, 1)
	go func() { done <- b.runOnce(ctx) }()
	select {
	case ok := <-result:
		if !ok {
			t.Error("до ответа на hello отправлена преждевременная команда")
		}
	case <-time.After(3 * time.Second):
		t.Error("проверка порядка сигнализации не завершилась")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("проверочный клиент не остановился")
	}
}

func TestСовместимаяРезервнаяВерсияSDK(t *testing.T) {
	if cfg := finishConfig(TMConfig{}); cfg.SDKVersion != "6.0.0" {
		t.Fatal("резервная версия SDK отличается от проверенной клиентской")
	}
}
