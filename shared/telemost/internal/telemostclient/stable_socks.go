// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
	"whitelist-bypass/relay/bond"
)

// Внешний SOCKS принадлежит всему запуску клиента, а не одной эпохе сервера.
// Реальный сброс серверной эпохи закрывает старые TCP, но не локальный порт.
func runStableConfigured(ctx context.Context, address string, links []string) error {
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	backend := reservation.Addr().String()
	reservation.Close()
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- runPersistentSOCKSFront(work, address, backend) }()
	go func() {
		results <- RunSelectiveLinksDiagnostics(work, backend, bond.DefaultSelectiveProfile(), links, nil)
	}()
	result := <-results
	cancel()
	<-results
	return result
}

func waitSOCKSBackend(ctx context.Context, backend string) error {
	for {
		d := net.Dialer{Timeout: 250 * time.Millisecond}
		conn, err := d.DialContext(ctx, "tcp", backend)
		if err == nil {
			conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
			_, err = conn.Write([]byte{5, 1, 0})
			var reply [2]byte
			if err == nil {
				_, err = io.ReadFull(conn, reply[:])
			}
			conn.Close()
			if err == nil && reply == [2]byte{5, 0} {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func runPersistentSOCKSFront(ctx context.Context, address, backend string) error {
	// До первой настоящей готовности транспорт не объявляет локальный SOCKS.
	if err := waitSOCKSBackend(ctx, backend); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		cancel()
		return fmt.Errorf("SplitYTM: постоянный SOCKS: %w", err)
	}
	defer listener.Close()
	stop := context.AfterFunc(ctx, func() { listener.Close() })
	defer stop()
	log.Printf("SplitYTM: постоянный SOCKS %s; смена серверной сессии не закрывает порт", address)
	limit := make(chan struct{}, 128)
	var workers sync.WaitGroup
	defer workers.Wait()
	defer cancel()
	for {
		front, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case limit <- struct{}{}:
		default:
			front.Close()
			continue
		}
		workers.Add(1)
		go func() { defer workers.Done(); defer func() { <-limit }(); forwardSOCKSFront(ctx, front, backend) }()
	}
}

func forwardSOCKSFront(ctx context.Context, front net.Conn, backend string) {
	defer front.Close()
	stop := context.AfterFunc(ctx, func() { front.Close() })
	defer stop()
	wait, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var up net.Conn
	for wait.Err() == nil {
		var err error
		d := net.Dialer{Timeout: 300 * time.Millisecond}
		up, err = d.DialContext(wait, "tcp", backend)
		if err == nil {
			break
		}
		select {
		case <-wait.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	if up == nil {
		return
	}
	defer up.Close()
	stopUp := context.AfterFunc(ctx, func() { up.Close() })
	defer stopUp()
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		io.Copy(up, front)
		if tcp, ok := up.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()
	io.Copy(front, up)
	front.Close()
	up.Close()
	<-sent
}
