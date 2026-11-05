// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

//go:build linux

// Защита служебных сокетов SplitYTM от повторного попадания в собственный TUN.
package netprotect

import (
	"context"
	"fmt"
	"github.com/pion/transport/v4"
	"github.com/pion/transport/v4/stdnet"
	"golang.org/x/sys/unix"
	"net"
	"syscall"
	"time"
)

var device string

func Configure(name string) error {
	if name != "" {
		i, e := net.InterfaceByName(name)
		if e != nil {
			return e
		}
		if i.Flags&net.FlagLoopback != 0 {
			return fmt.Errorf("SplitYTM: loopback не может быть внешним интерфейсом")
		}
	}
	device = name
	return nil
}
func Interface() string { return device }
func Control(network, address string, raw syscall.RawConn) error {
	if device == "" {
		return nil
	}
	host, _, _ := net.SplitHostPort(address)
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	var e error
	if err := raw.Control(func(fd uintptr) { e = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, device) }); err != nil {
		return err
	}
	if e != nil {
		return fmt.Errorf("SplitYTM: привязка сокета к %s: %w", device, e)
	}
	return nil
}
func ProtectDialer(d *net.Dialer) {
	previous := d.Control
	d.Control = func(n, a string, c syscall.RawConn) error {
		if previous != nil {
			if e := previous(n, a, c); e != nil {
				return e
			}
		}
		return Control(n, a, c)
	}
}
func ProtectListenConfig(l *net.ListenConfig) {
	previous := l.Control
	l.Control = func(n, a string, c syscall.RawConn) error {
		if previous != nil {
			if e := previous(n, a, c); e != nil {
				return e
			}
		}
		return Control(n, a, c)
	}
}
func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d := net.Dialer{Timeout: 10 * time.Second}
	ProtectDialer(&d)
	return d.DialContext(ctx, network, address)
}

type protectedNet struct{ transport.Net }

func NewNet() (transport.Net, error) {
	base, e := stdnet.NewNet()
	if e != nil {
		return nil, e
	}
	return &protectedNet{base}, nil
}
func (n *protectedNet) ListenPacket(network, address string) (net.PacketConn, error) {
	l := net.ListenConfig{}
	ProtectListenConfig(&l)
	return l.ListenPacket(context.Background(), network, address)
}
func (n *protectedNet) ListenUDP(network string, address *net.UDPAddr) (transport.UDPConn, error) {
	a := ""
	if address != nil {
		a = address.String()
	}
	c, e := n.ListenPacket(network, a)
	if e != nil {
		return nil, e
	}
	u, ok := c.(*net.UDPConn)
	if !ok {
		c.Close()
		return nil, fmt.Errorf("SplitYTM: ожидался UDP-сокет")
	}
	return u, nil
}
func (n *protectedNet) Dial(network, address string) (net.Conn, error) {
	return DialContext(context.Background(), network, address)
}
func (n *protectedNet) DialUDP(network string, local, remote *net.UDPAddr) (transport.UDPConn, error) {
	d := net.Dialer{LocalAddr: local}
	ProtectDialer(&d)
	c, e := d.Dial(network, remote.String())
	if e != nil {
		return nil, e
	}
	return c.(*net.UDPConn), nil
}
func (n *protectedNet) DialTCP(network string, local, remote *net.TCPAddr) (transport.TCPConn, error) {
	d := net.Dialer{LocalAddr: local}
	ProtectDialer(&d)
	c, e := d.Dial(network, remote.String())
	if e != nil {
		return nil, e
	}
	return c.(*net.TCPConn), nil
}
func (n *protectedNet) CreateDialer(d *net.Dialer) transport.Dialer {
	copy := *d
	ProtectDialer(&copy)
	return &copy
}
func (n *protectedNet) CreateListenConfig(l *net.ListenConfig) transport.ListenConfig {
	copy := *l
	ProtectListenConfig(&copy)
	return &copy
}
