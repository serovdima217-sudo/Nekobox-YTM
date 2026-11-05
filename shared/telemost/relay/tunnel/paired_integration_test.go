// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package tunnel_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"whitelist-bypass/relay/bond"
	"whitelist-bypass/relay/common"
	"whitelist-bypass/relay/tunnel"
)

type pairedPipe struct {
	mu     sync.Mutex
	onData func([]byte)
	peer   *pairedPipe
	queue  chan []byte
	stop   <-chan struct{}
	lost   atomic.Bool
}

func (w *pairedPipe) SendData(p []byte) {
	if w.lost.Load() {
		return
	}
	p = append([]byte{}, p...)
	select {
	case w.queue <- p:
	case <-w.stop:
	}
}
func (w *pairedPipe) SetOnData(fn func([]byte)) { w.mu.Lock(); w.onData = fn; w.mu.Unlock() }
func (*pairedPipe) SetOnClose(func())           {}
func (*pairedPipe) Reconfigure(int, int)        {}
func (w *pairedPipe) run() {
	for {
		select {
		case <-w.stop:
			return
		case p := <-w.queue:
			if w.lost.Load() {
				continue
			}
			w.peer.mu.Lock()
			fn := w.peer.onData
			w.peer.mu.Unlock()
			if fn != nil {
				fn(p)
			}
		}
	}
}

func TestТотЖеTCPЧерезSOCKSПереживаетПотерюОсновных(t *testing.T) {
	reliableTCPTest(t, false)
}
func TestТотЖеTCPПереходитНаОблегчённыйРезерв(t *testing.T) {
	reliableTCPTest(t, true)
}
func reliableTCPTest(t *testing.T, selective bool) {
	oldLoopback := common.AllowLoopbackDst
	common.AllowLoopbackDst = true
	t.Cleanup(func() { common.AllowLoopbackDst = oldLoopback })
	stop := make(chan struct{})
	defer close(stop)
	key := sha256.Sum256([]byte("настоящий TCP через четыре испытательных провода"))
	server, client := bond.NewPaired(key, true, nil, 80*time.Millisecond), bond.NewPaired(key, false, nil, 80*time.Millisecond)
	width := 4
	if selective {
		server.Close()
		client.Close()
		server = bond.NewSelective(key, true, nil)
		client = bond.NewSelective(key, false, nil)
		width = 2
	}
	defer server.Close()
	defer client.Close()
	var a, b [4]*pairedPipe
	for i := 0; i < width; i++ {
		i := i
		a[i] = &pairedPipe{queue: make(chan []byte, 64), stop: stop}
		b[i] = &pairedPipe{queue: make(chan []byte, 64), stop: stop}
		a[i].peer = b[i]
		b[i].peer = a[i]
		a[i].SetOnData(func(p []byte) { server.Handle(i, a[i], p) })
		b[i].SetOnData(func(p []byte) { client.Handle(i, b[i], p) })
		go a[i].run()
		go b[i].run()
		server.Attach(i, a[i])
		client.Attach(i, b[i])
	}
	for _, tr := range []*bond.Transport{server, client} {
		select {
		case <-tr.Ready():
		case <-time.After(5 * time.Second):
			t.Fatal("нет четырёх путей")
		}
	}
	logger := func(string, ...any) {}
	rbServer := tunnel.NewRelayBridge(server, "creator", 32768, logger)
	defer rbServer.Close()
	rbClient := tunnel.NewRelayBridge(client, "joiner", 32768, logger)
	defer rbClient.Close()
	rbClient.MarkReady()
	source, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer source.Close()
	var accepts atomic.Int32
	const size = 4 << 20
	pattern := bytes.Repeat([]byte("точная передача одного TCP"), 1024)
	want := make([]byte, size)
	for i := range want {
		want[i] = pattern[i%len(pattern)]
	}
	sourceDone := make(chan error, 1)
	go func() {
		c, e := source.Accept()
		if e != nil {
			sourceDone <- e
			return
		}
		accepts.Add(1)
		defer c.Close()
		c.SetDeadline(time.Now().Add(15 * time.Second))
		for offset := 0; offset < len(want); offset += 16384 {
			end := offset + 16384
			if end > len(want) {
				end = len(want)
			}
			if _, e = c.Write(want[offset:end]); e != nil {
				sourceDone <- e
				return
			}
			time.Sleep(time.Millisecond)
		}
		sourceDone <- nil
	}()
	// Порт выделяется ОС; повторная привязка нужна только для существующего API ListenSOCKS.
	probe, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	proxyAddr := probe.Addr().String()
	probe.Close()
	listenDone := make(chan error, 1)
	go func() { listenDone <- rbClient.ListenSOCKS(proxyAddr) }()
	var conn net.Conn
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		conn, e = net.DialTimeout("tcp", proxyAddr, 50*time.Millisecond)
		if e == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	if _, e = conn.Write([]byte{5, 1, 0}); e != nil {
		t.Fatal(e)
	}
	greeting := make([]byte, 2)
	if _, e = io.ReadFull(conn, greeting); e != nil || !bytes.Equal(greeting, []byte{5, 0}) {
		t.Fatal("SOCKS не согласован")
	}
	port := source.Addr().(*net.TCPAddr).Port
	request := []byte{5, 1, 0, 1, 127, 0, 0, 1, 0, 0}
	binary.BigEndian.PutUint16(request[8:], uint16(port))
	conn.Write(request)
	response := make([]byte, 10)
	if _, e = io.ReadFull(conn, response); e != nil || response[1] != 0 {
		t.Fatalf("SOCKS не открыл источник: ответ %x, ошибка %v", response, e)
	}
	got := make([]byte, size)
	if _, e = io.ReadFull(conn, got[:512<<10]); e != nil {
		t.Fatal(e)
	}
	// Полная потеря двух основных путей в обе стороны посреди того же соединения.
	failCount := 2
	if selective {
		failCount = 1
	}
	for i := 0; i < failCount; i++ {
		a[i].lost.Store(true)
		b[i].lost.Store(true)
		server.Detach(i, a[i])
		client.Detach(i, b[i])
	}
	if _, e = io.ReadFull(conn, got[512<<10:]); e != nil {
		t.Fatalf("TCP оборвался вместо восстановления: %v", e)
	}
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Fatal("контрольная сумма не совпала")
	}
	if accepts.Load() != 1 {
		t.Fatal("источник получил новое TCP-соединение")
	}
	if server.Err() != nil || client.Err() != nil {
		t.Fatal("транспорт был пересоздан")
	}
	if e = <-sourceDone; e != nil {
		t.Fatal(e)
	}
	rbClient.Close()
	select {
	case e = <-listenDone:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal(fmt.Sprint("SOCKS не закрылся"))
	}
}
