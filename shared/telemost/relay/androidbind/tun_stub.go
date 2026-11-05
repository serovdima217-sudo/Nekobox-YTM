// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

//go:build !android

package androidbind

import "fmt"

func StartTun2Socks(fd, mtu, socksPort int, socksUser, socksPass string) error {
	return fmt.Errorf("tun2socks is only available on Android")
}

func StopTun2Socks() {}
