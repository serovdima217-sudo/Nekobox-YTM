// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func testSOCKSBackend(t *testing.T, ctx context.Context, address string) net.Listener {
	t.Helper()
	l, e := net.Listen("tcp", address)
	if e != nil {
		t.Fatal(e)
	}
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				stop := context.AfterFunc(ctx, func() { c.Close() })
				defer stop()
				var g [3]byte
				if _, e := io.ReadFull(c, g[:]); e != nil {
					return
				}
				if g != [3]byte{5, 1, 0} {
					return
				}
				c.Write([]byte{5, 0})
				io.Copy(c, c)
			}()
		}
	}()
	return l
}
func testFreeSOCKSAddress(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	a := l.Addr().String()
	l.Close()
	return a
}
func TestDesktopSOCKSPortSurvivesBackendRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	backendAddress := testFreeSOCKSAddress(t)
	frontAddress := testFreeSOCKSAddress(t)
	result := make(chan error, 1)
	go func() { result <- runPersistentSOCKSFront(ctx, frontAddress, backendAddress) }()
	time.Sleep(50 * time.Millisecond)
	if c, e := net.DialTimeout("tcp", frontAddress, 100*time.Millisecond); e == nil {
		c.Close()
		t.Fatal("SOCKS объявлен до готовности транспорта")
	}
	b := testSOCKSBackend(t, ctx, backendAddress)
	var first net.Conn
	var e error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		first, e = net.DialTimeout("tcp", frontAddress, 100*time.Millisecond)
		if e == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if first == nil {
		b.Close()
		t.Fatal("Постоянный SOCKS не запущен")
	}
	first.SetDeadline(time.Now().Add(time.Second))
	first.Write([]byte{5, 1, 0})
	var reply [2]byte
	if _, e = io.ReadFull(first, reply[:]); e != nil || reply != [2]byte{5, 0} {
		t.Fatal("Начальный SOCKS не готов", e)
	}
	first.Close()
	b.Close()
	second, e := net.DialTimeout("tcp", frontAddress, time.Second)
	if e != nil {
		t.Fatal("При смене эпохи локальный порт закрыт", e)
	}
	defer second.Close()
	second.SetDeadline(time.Now().Add(2 * time.Second))
	second.Write([]byte{5, 1, 0})
	time.Sleep(150 * time.Millisecond)
	b = testSOCKSBackend(t, ctx, backendAddress)
	defer b.Close()
	if _, e = io.ReadFull(second, reply[:]); e != nil || reply != [2]byte{5, 0} {
		t.Fatal("SOCKS не восстановился без смены локального порта", e)
	}
	second.Write([]byte("проверка"))
	body := make([]byte, len("проверка"))
	if _, e = io.ReadFull(second, body); e != nil || string(body) != "проверка" {
		t.Fatal("Передача после восстановления не работает", e)
	}
	second.Close()
	cancel()
	select {
	case e = <-result:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("SOCKS не остановился вместе с клиентом")
	}
}
