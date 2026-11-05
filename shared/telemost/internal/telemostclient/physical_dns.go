// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
	"whitelist-bypass/relay/netprotect"
)

// DNS служебного транспорта не должен получать fake-IP из собственного VPN.
// Это только физический DNS-bootstrap; протокол SplitYTM и каналы не меняются.
func configurePhysicalDNS() {
	transport := &http.Transport{
		Proxy:               nil,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 8 * time.Second,
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConns:        8,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if host == "common.dot.dns.yandex.net" {
				var last error
				for _, ip := range []string{"77.88.8.1", "77.88.8.8"} {
					conn, e := netprotect.DialContext(ctx, network, net.JoinHostPort(ip, port))
					if e == nil {
						return conn, nil
					}
					last = e
				}
				return nil, last
			}
			return nil, fmt.Errorf("неизвестный сервер физического DNS")
		},
	}
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second}
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		front, back := net.Pipe()
		go func() {
			defer back.Close()
			// net.Pipe не является PacketConn: Go использует стандартное TCP-обрамление DNS.
			var header [2]byte
			if _, err := io.ReadFull(back, header[:]); err != nil {
				return
			}
			size := int(binary.BigEndian.Uint16(header[:]))
			if size < 12 || size > 4096 {
				return
			}
			query := make([]byte, size)
			if _, err := io.ReadFull(back, query); err != nil {
				return
			}
			request, err := http.NewRequestWithContext(ctx, "POST", "https://common.dot.dns.yandex.net/dns-query", bytes.NewReader(query))
			if err != nil {
				return
			}
			request.Header.Set("Content-Type", "application/dns-message")
			request.Header.Set("Accept", "application/dns-message")
			response, err := client.Do(request)
			if err != nil {
				return
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return
			}
			answer, err := io.ReadAll(io.LimitReader(response.Body, 65536))
			if err != nil || len(answer) < 12 || len(answer) > 65535 {
				return
			}
			binary.BigEndian.PutUint16(header[:], uint16(len(answer)))
			if _, err = back.Write(header[:]); err != nil {
				return
			}
			_, _ = back.Write(answer)
		}()
		return front, nil
	}}
}
