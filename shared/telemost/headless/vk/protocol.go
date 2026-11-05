// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import "github.com/kulikov0/headless-client/webrtc"

type Relay interface {
	Init(iceServers []webrtc.ICEServer) error
	CreateOffer() (webrtc.SessionDescription, error)
	CreateAnswer() (webrtc.SessionDescription, error)
	SetRemoteDescription(sdpType webrtc.SDPType, sdp string) error
	AddICECandidate(candidate webrtc.ICECandidateInit) error
	OnICECandidate(fn func(*webrtc.ICECandidate))
	OnConnectionStateChange(fn func(webrtc.PeerConnectionState))
	Close()
}
