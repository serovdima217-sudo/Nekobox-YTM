// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"fmt"
	"strings"
	"testing"
	"whitelist-bypass/relay/bond"
)

func TestАвтоматическийПулСодержитПятьНепересекающихсяЧетвёрок(t *testing.T) {
	groups, e := automaticGroups(desktopTestInvitations(20))
	if e != nil || len(groups) != 5 {
		t.Fatal("неверный встроенный пул")
	}
	for _, g := range groups {
		if len(g) != bond.PairedLanes {
			t.Fatal("неполная четвёрка")
		}
	}
	if pairedFPS != 12 || pairedBatch != 40 {
		t.Fatal("профиль должен быть 12/40 вместо базового 24/40, а не 12/20")
	}
}
func TestПовторыВРазныхЧетвёркахЗапрещены(t *testing.T) {
	if _, e := automaticGroups(desktopTestInvitations(20) + desktopTestInvitations(20)); e == nil {
		t.Fatal("одна ссылка выдана в разные группы")
	}
}

func TestАвтоматическийВыборПятиНепересекающихсяПар(t *testing.T) {
	groups, e := selectiveGroups(desktopTestInvitations(10))
	if e != nil || len(groups) != 5 {
		t.Fatal("неверный встроенный пул пар")
	}
	for _, g := range groups {
		if len(g) != 2 {
			t.Fatal("неполная пара")
		}
	}
}

// У настольной сборки реальные приглашения приходят только из закрытой подписки.
func desktopTestInvitations(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "https://telemost.yandex.ru/j/%d\n", 100000+i)
	}
	return b.String()
}
func TestDesktopNoEmbeddedPrivateInvitations(t *testing.T) {
	if strings.Contains(automaticInvitations, "telemost.yandex") || strings.Contains(selectiveInvitations, "telemost.yandex") {
		t.Fatal("Приглашения не должны встраиваться в настольный бинарник")
	}
}
