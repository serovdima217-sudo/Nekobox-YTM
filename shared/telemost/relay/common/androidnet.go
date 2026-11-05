// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package common

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"

	"github.com/pion/transport/v4"
)

func init() {
	if runtime.GOOS != "android" || os.Getenv("SSL_CERT_DIR") != "" {
		return
	}
	// На Android 14+ корневые сертификаты находятся в обновляемом модуле Conscrypt.
	var directories []string
	for _, path := range []string{"/apex/com.android.conscrypt/cacerts", "/system/etc/security/cacerts"} {
		if stat, err := os.Stat(path); err == nil && stat.IsDir() {
			directories = append(directories, path)
		}
	}
	if len(directories) > 0 {
		os.Setenv("SSL_CERT_DIR", strings.Join(directories, ":"))
	}
}

// AndroidNet implements transport.Net without netlink interface enumeration
type AndroidNet struct {
	LocalIP string
}

func (n *AndroidNet) ListenPacket(network string, address string) (net.PacketConn, error) {
	return net.ListenPacket(network, address)
}

func (n *AndroidNet) ListenUDP(network string, locAddr *net.UDPAddr) (transport.UDPConn, error) {
	conn, err := net.ListenUDP(network, locAddr)
	if err != nil {
		return nil, err
	}
	return WrapBatchUDP(conn), nil
}

type wrappedTCPListener struct {
	*net.TCPListener
}

func (w *wrappedTCPListener) AcceptTCP() (transport.TCPConn, error) {
	return w.TCPListener.AcceptTCP()
}

func (n *AndroidNet) ListenTCP(network string, laddr *net.TCPAddr) (transport.TCPListener, error) {
	l, err := net.ListenTCP(network, laddr)
	if err != nil {
		return nil, err
	}
	return &wrappedTCPListener{l}, nil
}

func (n *AndroidNet) Dial(network, address string) (net.Conn, error) {
	return net.Dial(network, address)
}

func (n *AndroidNet) DialUDP(network string, laddr, raddr *net.UDPAddr) (transport.UDPConn, error) {
	return net.DialUDP(network, laddr, raddr)
}

func (n *AndroidNet) DialTCP(network string, laddr, raddr *net.TCPAddr) (transport.TCPConn, error) {
	return net.DialTCP(network, laddr, raddr)
}

func (n *AndroidNet) ResolveIPAddr(network, address string) (*net.IPAddr, error) {
	return net.ResolveIPAddr(network, address)
}

func (n *AndroidNet) ResolveUDPAddr(network, address string) (*net.UDPAddr, error) {
	return net.ResolveUDPAddr(network, address)
}

func (n *AndroidNet) ResolveTCPAddr(network, address string) (*net.TCPAddr, error) {
	return net.ResolveTCPAddr(network, address)
}

func (n *AndroidNet) Interfaces() ([]*transport.Interface, error) {
	// На Android перечисление интерфейсов через netlink запрещено обычному UID.
	// Явный LocalIP также включает этот путь для воспроизводимого теста Linux.
	if runtime.GOOS == "android" || n.LocalIP != "" {
		return n.scanSysNet()
	}
	ifaces, err := net.Interfaces()
	if err == nil && len(ifaces) > 0 {
		result := make([]*transport.Interface, len(ifaces))
		for i := range ifaces {
			ifc := transport.NewInterface(ifaces[i])
			addrs, addrErr := ifaces[i].Addrs()
			if addrErr == nil {
				for _, addr := range addrs {
					ifc.AddAddress(addr)
				}
			}
			result[i] = ifc
		}
		return result, nil
	}
	return n.scanSysNet()
}

func (n *AndroidNet) scanSysNet() ([]*transport.Interface, error) {
	if n.LocalIP == "" {
		// UDP connect выбирает исходный адрес ядром, но не отправляет пакет.
		// Проверяем оба семейства, не обращаясь к AF_NETLINK.
		var result []*transport.Interface
		for index, target := range []struct{ network, address string }{
			{"udp4", "77.88.7.57:443"}, {"udp6", "[2a02:6b8::2:242]:443"},
		} {
			conn, err := net.Dial(target.network, target.address)
			if err != nil {
				continue
			}
			addr, ok := conn.LocalAddr().(*net.UDPAddr)
			conn.Close()
			if !ok || addr.IP.IsUnspecified() || addr.IP.IsLoopback() {
				continue
			}
			next := &AndroidNet{LocalIP: addr.IP.String()}
			items, err := next.scanSysNet()
			if err != nil {
				continue
			}
			for _, item := range items {
				item.Index = index + 1
				item.Name = fmt.Sprintf("android%d", index)
				result = append(result, item)
			}
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("не выбран исходный адрес Android без netlink")
		}
		return result, nil
	}
	ip := net.ParseIP(n.LocalIP)
	if ip == nil {
		return nil, fmt.Errorf("неверный исходный IP-адрес Android")
	}

	iface := net.Interface{
		Index:        1,
		MTU:          1500,
		Name:         "rmnet0",
		Flags:        net.FlagUp | net.FlagMulticast,
		HardwareAddr: net.HardwareAddr{0, 0, 0, 0, 0, 0},
	}

	ones := 32
	if ip.To4() == nil {
		ones = 128
	}
	ti := transport.NewInterface(iface)
	ti.AddAddress(&net.IPNet{IP: ip, Mask: net.CIDRMask(ones, ones)})

	return []*transport.Interface{ti}, nil
}

func (n *AndroidNet) InterfaceByIndex(index int) (*transport.Interface, error) {
	ifaces, err := n.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		if iface.Index == index {
			return iface, nil
		}
	}
	return nil, fmt.Errorf("interface index %d not found", index)
}

func (n *AndroidNet) InterfaceByName(name string) (*transport.Interface, error) {
	ifaces, err := n.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		if iface.Name == name {
			return iface, nil
		}
	}
	return nil, fmt.Errorf("interface %s not found", name)
}

func (n *AndroidNet) CreateDialer(dialer *net.Dialer) transport.Dialer {
	return dialer
}

func (n *AndroidNet) CreateListenConfig(lc *net.ListenConfig) transport.ListenConfig {
	return lc
}
