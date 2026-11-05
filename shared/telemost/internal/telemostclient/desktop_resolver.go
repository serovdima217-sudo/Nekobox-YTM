// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// Никаких адресов VPS или приглашений: только публичные DNS-записи служб Яндекса.
// TLS по-прежнему проверяет исходное имя. Системный DNS не меняется.
//
//go:embed bootstrap_dns.json
var desktopBootstrapData []byte
var desktopDNSMu sync.Mutex

type desktopDNSEntry struct {
	ip    string
	until time.Time
}

var desktopDNSCache = map[string]desktopDNSEntry{}

func desktopLookup(host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	desktopDNSMu.Lock()
	cached := desktopDNSCache[host]
	desktopDNSMu.Unlock()
	if cached.ip != "" && time.Now().Before(cached.until) {
		return cached.ip, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	cancel()
	if err == nil && len(ips) > 0 {
		value := ips[0].String()
		desktopDNSMu.Lock()
		desktopDNSCache[host] = desktopDNSEntry{value, time.Now().Add(time.Minute)}
		desktopDNSMu.Unlock()
		return value, nil
	}
	var bootstrap map[string][]string
	if json.Unmarshal(desktopBootstrapData, &bootstrap) == nil && len(bootstrap[host]) > 0 {
		return bootstrap[host][0], nil
	}
	// Отдельный срок для IPv6: истёкшее ожидание IPv4 не отменяет его заранее.
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if ips, err = net.DefaultResolver.LookupIP(ctx, "ip6", host); err == nil && len(ips) > 0 {
		return ips[0].String(), nil
	}
	return "", fmt.Errorf("не найден IP для %s", host)
}
