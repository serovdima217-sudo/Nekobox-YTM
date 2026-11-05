// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

//go:build linux

package netprotect

import (
	"golang.org/x/sys/unix"
	"net"
	"testing"
)

func TestRejectInvalidInterface(t *testing.T) {
	if Configure("nekobox-no-such-interface") == nil {
		t.Fatal("Принят несуществующий интерфейс")
	}
}
func TestUnprivilegedUDPSocketBinding(t *testing.T) {
	previous := device
	device = "lo"
	defer func() { device = previous }()
	n, e := NewNet()
	if e != nil {
		t.Fatal(e)
	}
	u, e := n.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if e != nil {
		t.Fatal(e)
	}
	defer u.Close()
	raw, e := u.(*net.UDPConn).SyscallConn()
	if e != nil {
		t.Fatal(e)
	}
	var bound string
	var err error
	_ = raw.Control(func(fd uintptr) { bound, err = unix.GetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE) })
	if err != nil || bound != "lo" {
		t.Fatal("Сокет не привязан к выбранному интерфейсу:", bound, err)
	}
}
