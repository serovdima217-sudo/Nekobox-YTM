// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

//go:build !android && !linux

package headlessapi

import "github.com/kulikov0/headless-client/webrtc"

func configurePlatformNetwork(e *webrtc.SettingEngine) {}
