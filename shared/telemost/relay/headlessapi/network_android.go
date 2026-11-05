// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

//go:build android

package headlessapi

import (
	"github.com/kulikov0/headless-client/webrtc"
	"whitelist-bypass/relay/common"
)

// Не используем stdnet.NewNet: Android запрещает его netlink-запросы без root.
func configurePlatformNetwork(e *webrtc.SettingEngine) { e.SetNet(&common.AndroidNet{}) }
