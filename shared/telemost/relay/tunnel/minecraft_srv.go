// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package tunnel

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"whitelist-bypass/relay/common"
)

type srvCacheEntry struct {
	target  string
	expires time.Time
}

var minecraftSRVCache struct {
	sync.Mutex
	items map[string]srvCacheEntry
}

var srvLookupFunc = func(ctx context.Context, host string) ([]*net.SRV, error) {
	_, records, err := net.DefaultResolver.LookupSRV(ctx, "minecraft", "tcp", host)
	return records, err
}

// resolveMinecraftSRV проверяет запись _minecraft._tcp.<host> при подключении к стандартному
// порту 25565, когда клиент в режиме FakeDNS не может получить SRV самостоятельно.
func resolveMinecraftSRV(addr string, logFn func(string, ...any)) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port != "25565" || host == "" || net.ParseIP(host) != nil {
		return addr
	}
	cleanHost := strings.ToLower(strings.TrimSuffix(host, "."))
	if cleanHost == "" || strings.HasPrefix(cleanHost, "_") {
		return addr
	}

	now := time.Now()
	minecraftSRVCache.Lock()
	if minecraftSRVCache.items != nil {
		if item, ok := minecraftSRVCache.items[cleanHost]; ok && now.Before(item.expires) {
			minecraftSRVCache.Unlock()
			if item.target != "" {
				return item.target
			}
			return addr
		}
	}
	minecraftSRVCache.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	records, lookupErr := srvLookupFunc(ctx, cleanHost)
	cancel()

	resolved := ""
	ttl := 30 * time.Second
	if lookupErr == nil && len(records) > 0 {
		for _, rec := range records {
			targetHost := strings.TrimSuffix(rec.Target, ".")
			if targetHost != "" && rec.Port > 0 {
				candidate := net.JoinHostPort(targetHost, strconv.Itoa(int(rec.Port)))
				if !common.DstBlocked(candidate) {
					resolved = candidate
					ttl = 60 * time.Second
					break
				}
			}
		}
	}

	minecraftSRVCache.Lock()
	if minecraftSRVCache.items == nil {
		minecraftSRVCache.items = make(map[string]srvCacheEntry)
	}
	if len(minecraftSRVCache.items) > 1024 {
		for k, v := range minecraftSRVCache.items {
			if now.After(v.expires) {
				delete(minecraftSRVCache.items, k)
			}
		}
	}
	minecraftSRVCache.items[cleanHost] = srvCacheEntry{target: resolved, expires: now.Add(ttl)}
	minecraftSRVCache.Unlock()

	if resolved != "" && resolved != addr {
		if logFn != nil {
			logFn("relay: Minecraft SRV %s -> %s", common.MaskAddr(addr), common.MaskAddr(resolved))
		}
		return resolved
	}
	return addr
}
