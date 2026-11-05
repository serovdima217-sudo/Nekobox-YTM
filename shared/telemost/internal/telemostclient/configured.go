// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"runtime/debug"
	"whitelist-bypass/relay/netprotect"
)

// ClientConfig не содержит cookie, ключей SSH и адреса VPS.
type ClientConfig struct {
	Type          string   `json:"type"`
	Name          string   `json:"name"`
	Links         []string `json:"links"`
	Listen        string   `json:"listen"`
	BindInterface string   `json:"bind_interface,omitempty"`
}

func RunConfigured(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c ClientConfig
	if err = json.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("неверный JSON клиента: %w", err)
	}
	if c.Type != "" && c.Type != "tm-split" {
		return fmt.Errorf("неподдерживаемый протокол")
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:9999"
	}
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return fmt.Errorf("неверный локальный адрес SOCKS")
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("SOCKS должен слушать только loopback")
	}
	if len(c.Links) > 50 {
		return fmt.Errorf("в подписке допускается не более 50 ссылок")
	}
	if _, err = selectiveGroups(stringsJoin(c.Links)); err != nil {
		return err
	}
	if err = netprotect.Configure(c.BindInterface); err != nil {
		return err
	}
	if c.BindInterface != "" {
		log.Printf("SplitYTM: служебные TCP/UDP-сокеты привязаны к %s, мимо TUN", c.BindInterface)
		net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: netprotect.DialContext}
	}
	debug.SetMemoryLimit(128 << 20)
	if err = configureDNS(""); err != nil {
		return err
	}
	if c.BindInterface != "" {
		configurePhysicalDNS()
	}
	return runStableConfigured(ctx, c.Listen, c.Links)
}
func stringsJoin(links []string) string {
	result := ""
	for _, s := range links {
		result += s + "\n"
	}
	return result
}
