// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package common

import (
	"errors"
	"net"
	"net/netip"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pion/transport/v4"
	"golang.org/x/net/ipv4"
)

// BatchNet меняет только отправку UDP; обнаружение интерфейсов остаётся у базовой сети.
type BatchNet struct{ transport.Net }

func (n *BatchNet) ListenUDP(network string, addr *net.UDPAddr) (transport.UDPConn, error) {
	c, err := n.Net.ListenUDP(network, addr)
	if err != nil {
		return nil, err
	}
	if raw, ok := c.(*net.UDPConn); ok {
		return WrapBatchUDP(raw), nil
	}
	return c, nil
}

var udpSendBuffers = sync.Pool{New: func() any { return make([]byte, 2048) }}

type queuedUDP struct {
	data []byte
	addr netip.AddrPort
}
type batchUDP struct {
	*net.UDPConn
	packets  *ipv4.PacketConn
	queue    chan queuedUDP
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	closed   atomic.Bool
	deadline atomic.Int64
}

// Ограниченная очередь и один sendmmsg вместо отдельных sendto для пачки RTP.
// Задержка накопления не превышает одного короткого тика планировщика.
// IPv6 и системы без поддержки пакетной отправки сохраняют обычный путь.
func WrapBatchUDP(raw *net.UDPConn) transport.UDPConn {
	if os.Getenv("TM_SPLIT_UDP_BATCH") != "1" || (runtime.GOOS != "linux" && runtime.GOOS != "android") || raw.LocalAddr().(*net.UDPAddr).IP.To4() == nil {
		return raw
	}
	c := &batchUDP{UDPConn: raw, packets: ipv4.NewPacketConn(raw), queue: make(chan queuedUDP, 256), stop: make(chan struct{}), done: make(chan struct{})}
	go c.writer()
	return c
}
func (c *batchUDP) SetWriteDeadline(t time.Time) error {
	if t.IsZero() {
		c.deadline.Store(0)
	} else {
		c.deadline.Store(t.UnixNano())
	}
	return c.UDPConn.SetWriteDeadline(t)
}
func (c *batchUDP) SetDeadline(t time.Time) error {
	if err := c.SetWriteDeadline(t); err != nil {
		return err
	}
	return c.UDPConn.SetReadDeadline(t)
}
func (c *batchUDP) WriteTo(data []byte, addr net.Addr) (int, error) {
	if c.closed.Load() {
		return 0, net.ErrClosed
	}
	target, ok := addr.(*net.UDPAddr)
	if !ok || target == nil {
		return c.UDPConn.WriteTo(data, addr)
	}
	ip, ok := netip.AddrFromSlice(target.IP)
	if !ok || target.Port < 0 || target.Port > 65535 || len(data) > 65507 {
		return c.UDPConn.WriteTo(data, addr)
	}
	ip = ip.Unmap()
	if target.Zone != "" {
		ip = ip.WithZone(target.Zone)
	}
	buf := udpSendBuffers.Get().([]byte)
	if cap(buf) < len(data) {
		buf = make([]byte, len(data))
	}
	buf = buf[:len(data)]
	copy(buf, data)
	packet := queuedUDP{buf, netip.AddrPortFrom(ip, uint16(target.Port))}
	var timeout <-chan time.Time
	var timer *time.Timer
	if deadline := c.deadline.Load(); deadline != 0 {
		delta := time.Until(time.Unix(0, deadline))
		if delta <= 0 {
			udpSendBuffers.Put(buf[:0])
			return 0, os.ErrDeadlineExceeded
		}
		timer = time.NewTimer(delta)
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case c.queue <- packet:
		return len(data), nil
	case <-c.stop:
		udpSendBuffers.Put(buf[:0])
		return 0, net.ErrClosed
	case <-timeout:
		udpSendBuffers.Put(buf[:0])
		return 0, os.ErrDeadlineExceeded
	}
}
func (c *batchUDP) WriteToUDP(data []byte, addr *net.UDPAddr) (int, error) {
	return c.WriteTo(data, addr)
}
func (c *batchUDP) Close() error {
	var err error
	c.once.Do(func() { c.closed.Store(true); close(c.stop); err = c.UDPConn.Close(); <-c.done })
	return err
}
func (c *batchUDP) writer() {
	defer close(c.done)
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	packets := make([]queuedUDP, 0, 32)
	messages := make([]ipv4.Message, 32)
	buffers := make([][]byte, 32)
	for i := range messages {
		messages[i].Buffers = buffers[i : i+1]
	}
	addresses := make(map[netip.AddrPort]*net.UDPAddr)
	plain := false
	defer func() {
		for {
			select {
			case packet := <-c.queue:
				udpSendBuffers.Put(packet.data[:0])
			default:
				return
			}
		}
	}()
	for {
		var first queuedUDP
		select {
		case <-c.stop:
			return
		case first = <-c.queue:
		}
		packets = append(packets[:0], first)
		timer.Reset(150 * time.Microsecond)
	collect:
		for len(packets) < len(messages) {
			select {
			case next := <-c.queue:
				packets = append(packets, next)
			case <-timer.C:
				break collect
			case <-c.stop:
				break collect
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		for i, p := range packets {
			addr := addresses[p.addr]
			if addr == nil {
				addr = net.UDPAddrFromAddrPort(p.addr)
				if len(addresses) < 64 {
					addresses[p.addr] = addr
				}
			}
			messages[i].Addr = addr
			messages[i].Buffers[0] = p.data
		}
		count := 0
		if !plain {
			for count < len(packets) {
				n, err := c.packets.WriteBatch(messages[count:len(packets)], 0)
				count += n
				if err != nil {
					if errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.ENOTSUP) {
						plain = true
					}
					break
				}
				if n == 0 {
					break
				}
			}
		}
		// Повторяем неотправленную часть обычными вызовами; порядок пакетов сохраняется.
		for i := count; i < len(packets); i++ {
			_, _ = c.UDPConn.WriteTo(packets[i].data, messages[i].Addr)
		}
		for i, p := range packets {
			udpSendBuffers.Put(p.data[:0])
			packets[i] = queuedUDP{}
			messages[i].Buffers[0] = nil
			messages[i].Addr = nil
		}
	}
}
