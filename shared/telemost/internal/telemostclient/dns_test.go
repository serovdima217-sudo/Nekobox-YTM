// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import "testing"

func TestАдресаDNS(t *testing.T) {
	for input, want := range map[string]string{"77.88.8.8": "77.88.8.8:53", "1.1.1.1:5353": "1.1.1.1:5353", "::1": "[::1]:53"} {
		got, err := dnsAddress(input)
		if err != nil || got != want {
			t.Fatal("неверная нормализация DNS", input)
		}
	}
	for _, input := range []string{"dns.example.com:53", "https://dns.example.com", "127.0.0.1:99999", ""} {
		if _, err := dnsAddress(input); err == nil {
			t.Fatal("принят неверный DNS", input)
		}
	}
}
