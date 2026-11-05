// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"whitelist-bypass/relay/bond"
)

func TestMetricsDirections(t *testing.T) {
	up, down := laneTotals([]bond.Stats{{Sent: 100, Received: 70}, {Sent: 40, Received: 10}})
	if up != 80 || down != 140 {
		t.Fatal("Направления счётчиков перепутаны")
	}
}
func TestMetricsKeepTotalsAfterEpoch(t *testing.T) {
	p, err := newSelectivePool([]string{"https://telemost.yandex.ru/j/12345", "https://telemost.yandex.ru/j/67890"}, bond.DefaultSelectiveProfile())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.mu.Lock()
	p.groups[0].previousUp = 123
	p.groups[0].previousDown = 456
	p.groups[0].transport.Close()
	p.restartLocked(0)
	p.mu.Unlock()
	m := p.metrics("тест", true)
	if m.Pairs[0].Up != 123 || m.Pairs[0].Down != 456 || m.Active != 0 || m.Capacity != 1 {
		t.Fatal("Сброшены накопленные счётчики пары")
	}
}
func TestMetricsFilePrivateAndContainsNoInvitations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	m := splitMetrics{Schema: 1, Protocol: "SplitYTM", Instance: "тест"}
	if err := writeSplitMetrics(path, m); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"https://", "cookie", "owner", "room_id"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("Секреты в статистике")
		}
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal("Неверные права статистики")
	}
}
func TestMetricsStopWritesOfflineState(t *testing.T) {
	p, err := newSelectivePool([]string{"https://telemost.yandex.ru/j/12345", "https://telemost.yandex.ru/j/67890"}, bond.DefaultSelectiveProfile())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	path := filepath.Join(t.TempDir(), "stats.json")
	stop := startSplitMetrics(context.Background(), p, path)
	stop()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m splitMetrics
	if json.Unmarshal(b, &m) != nil || m.Running {
		t.Fatal("После остановки показано работающее соединение")
	}
}
