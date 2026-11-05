// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

func dnsAddress(value string) (string, error) {
	value = strings.TrimSpace(value)
	if ip := net.ParseIP(value); ip != nil {
		return net.JoinHostPort(ip.String(), "53"), nil
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || net.ParseIP(host) == nil || port == "" {
		return "", fmt.Errorf("DNS должен быть IP-адресом с необязательным портом: %s", value)
	}
	if _, err := net.LookupPort("udp", port); err != nil {
		return "", fmt.Errorf("неверный порт DNS")
	}
	return net.JoinHostPort(host, port), nil
}

func configureDNS(explicit string) error {
	if runtime.GOOS != "android" && explicit == "" {
		return nil
	}
	prefix := os.Getenv("PREFIX")
	if prefix == "" {
		prefix = "/data/data/com.termux/files/usr"
	}
	// В Android 14+ системные сертификаты также могут находиться в APEX.
	if runtime.GOOS == "android" {
		if os.Getenv("SSL_CERT_FILE") == "" {
			for _, name := range []string{"etc/tls/cert.pem", "etc/ssl/cert.pem"} {
				path := filepath.Join(prefix, name)
				if _, err := os.Stat(path); err == nil {
					os.Setenv("SSL_CERT_FILE", path)
					break
				}
			}
		}
		if os.Getenv("SSL_CERT_DIR") == "" {
			os.Setenv("SSL_CERT_DIR", "/apex/com.android.conscrypt/cacerts:/system/etc/security/cacerts:"+filepath.Join(prefix, "etc/ssl/certs"))
		}
	}
	var servers []string
	if explicit != "" {
		for _, value := range strings.Split(explicit, ",") {
			addr, err := dnsAddress(value)
			if err != nil {
				return err
			}
			servers = append(servers, addr)
		}
	} else {
		// Статическая сборка не обращается к несуществующему Android /etc/resolv.conf.
		if data, err := os.ReadFile(filepath.Join(prefix, "etc/resolv.conf")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 && fields[0] == "nameserver" {
					if addr, err := dnsAddress(fields[1]); err == nil {
						servers = append(servers, addr)
					}
				}
			}
		}
		if len(servers) == 0 {
			servers = []string{"77.88.8.8:53", "77.88.8.1:53"}
		}
	}
	var next atomic.Uint32
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		addr := servers[int(next.Add(1)-1)%len(servers)]
		d := net.Dialer{Timeout: 3 * time.Second}
		return d.DialContext(ctx, network, addr)
	}}
	return nil
}
