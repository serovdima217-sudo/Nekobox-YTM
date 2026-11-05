// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

//go:build linux && !android

package headlessapi

import (
	"github.com/kulikov0/headless-client/webrtc"
	"github.com/pion/transport/v4/stdnet"
	"os"
	"whitelist-bypass/relay/common"
)

func configurePlatformNetwork(e *webrtc.SettingEngine) {
	if os.Getenv("TM_SPLIT_UDP_BATCH") != "1" {
		return
	}
	if n, err := stdnet.NewNet(); err == nil {
		e.SetNet(&common.BatchNet{Net: n})
	}
}
