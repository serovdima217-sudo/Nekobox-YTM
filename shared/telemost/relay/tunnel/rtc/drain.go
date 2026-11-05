// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package rtc

import (
	"whitelist-bypass/relay/common"

	"github.com/kulikov0/headless-client/webrtc"
)

func DrainSenderRTCP(sender *webrtc.RTPSender) {
	if sender == nil {
		return
	}
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func DrainTrack(track *webrtc.TrackRemote) {
	if track == nil {
		return
	}
	buf := make([]byte, common.UDPBufSize)
	for {
		if _, _, err := track.Read(buf); err != nil {
			return
		}
	}
}
