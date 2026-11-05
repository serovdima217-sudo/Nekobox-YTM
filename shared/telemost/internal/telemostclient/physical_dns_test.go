// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	"net"
	"os"
	"testing"
	"time"
	"whitelist-bypass/relay/netprotect"
)

func TestФизическийDNS(t *testing.T) {
	if os.Getenv("NEKOBOX_DNS_TEST") != "1" {
		t.Skip("Проверка настоящей сети запускается отдельно")
	}
	if err := netprotect.Configure(os.Getenv("NEKOBOX_DNS_INTERFACE")); err != nil {
		t.Fatal(err)
	}
	configurePhysicalDNS()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, "goloom.strm.yandex.net")
	if err != nil {
		t.Fatal("Не работает защищённый физический DNS:", err)
	}
	if len(addresses) == 0 {
		t.Fatal("Пустой ответ DNS")
	}
	_, fake, _ := net.ParseCIDR("198.18.0.0/15")
	for _, address := range addresses {
		if fake.Contains(address.IP) {
			t.Fatal("Служебному транспорту возвращён fake-IP")
		}
	}
	t.Log("Защищённый физический DNS возвращает настоящие адреса")
}
