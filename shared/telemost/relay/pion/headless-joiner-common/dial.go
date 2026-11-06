// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package joiner

import (
	"context"
	"net"
	"net/http"
	"time"
	"whitelist-bypass/relay/netprotect"

	"whitelist-bypass/relay/common"

	"github.com/kulikov0/headless-client"
)

func resolvingDialContext(resolveFn ResolveFunc) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if resolveFn == nil {
		return nil
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		resolvedIP, err := resolveFn(host)
		if err != nil {
			return nil, err
		}
		dialer := headless.ChromeDialer()
		dialer.Timeout = 10 * time.Second
		netprotect.ProtectDialer(dialer)
		return dialer.DialContext(ctx, network, net.JoinHostPort(resolvedIP, port))
	}
}

func resolvingHTTPClient(resolveFn ResolveFunc) *http.Client {
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: common.ChromeTransport(headless.TLSOptions{DialContext: resolvingDialContext(resolveFn)}),
	}
}
