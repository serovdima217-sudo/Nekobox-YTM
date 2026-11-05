// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

//go:build !ios && !darwin

package ios

import "fmt"

func StartTun2SocksPacketFlow(sink PacketFlowSink, mtu int, socksPort int, socksUser, socksPass string) error {
	return fmt.Errorf("tun2socks: iOS only")
}

func InputPacket(data []byte) {}

func StopTun2Socks() {}
