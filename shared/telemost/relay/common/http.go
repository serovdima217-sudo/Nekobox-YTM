// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package common

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"unsafe"

	"github.com/kulikov0/headless-client"
)

const bodySnippetLimit = 300

func BodySnippet(body []byte) string {
	if len(body) > bodySnippetLimit {
		return string(body[:bodySnippetLimit]) + "..."
	}
	return string(body)
}

func LoadCookies(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Cannot read cookies: %v", err)
	}
	var cookies []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &cookies); err != nil {
		log.Fatalf("Cannot parse cookies: %v", err)
	}
	parts := make([]string, len(cookies))
	for i, c := range cookies {
		parts[i] = c.Name + "=" + c.Value
	}
	return strings.Join(parts, "; ")
}

func UpdateCookieFile(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	seen := make(map[string]bool, len(updates))
	for _, c := range raw {
		name, _ := c["name"].(string)
		if v, ok := updates[name]; ok {
			c["value"] = v
			seen[name] = true
		}
	}
	for name, v := range updates {
		if !seen[name] {
			raw = append(raw, map[string]any{"name": name, "value": v})
		}
	}
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

func CookieValue(cookieHeader, name string) string {
	for _, part := range strings.Split(cookieHeader, ";") {
		part = strings.TrimSpace(part)
		eq := strings.IndexByte(part, '=')
		if eq != -1 && part[:eq] == name {
			return part[eq+1:]
		}
	}
	return ""
}

func FilterCookies(cookieHeader string, allow []string) string {
	allowed := make(map[string]struct{}, len(allow))
	for _, n := range allow {
		allowed[n] = struct{}{}
	}
	var out []string
	for _, part := range strings.Split(cookieHeader, ";") {
		trimmed := strings.TrimSpace(part)
		eq := strings.IndexByte(trimmed, '=')
		if eq == -1 {
			continue
		}
		if _, ok := allowed[trimmed[:eq]]; ok {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, "; ")
}

// ChromeTransport создаёт HTTP-транспорт с профилем браузера и безопасно инициализирует
// внутренний пул HTTP/2 при сборке компилятором Go 1.27+ без тега http2legacy.
func ChromeTransport(options headless.TLSOptions) http.RoundTripper {
	rt := headless.ChromeWindows.Transport(options)
	initEmbeddedHTTP2Transport(rt)
	return rt
}

// ChromeHTTPClient возвращает HTTP-клиент с инициализированным транспортом ChromeWindows.
func ChromeHTTPClient() *http.Client {
	c := headless.ChromeWindows.HTTPClient()
	initEmbeddedHTTP2Transport(c.Transport)
	return c
}

func initEmbeddedHTTP2Transport(rt http.RoundTripper) {
	if rt == nil {
		return
	}
	v := reflect.ValueOf(rt)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	f := elem.FieldByName("http2Transport")
	if !f.IsValid() || !f.CanAddr() {
		return
	}
	ptr := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr()))
	if closer, ok := ptr.Interface().(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
