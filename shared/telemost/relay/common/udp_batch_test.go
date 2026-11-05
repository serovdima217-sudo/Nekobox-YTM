// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package common

import (
	"bytes"
	"errors"
	"net"
	"os"
	"testing"
	"time"
)

func TestBatchUDPKeepsOrderAndCloses(t *testing.T) {
	t.Setenv("TM_SPLIT_UDP_BATCH", "1")
	receiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	raw, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	sender := WrapBatchUDP(raw)
	defer sender.Close()
	for i := 0; i < 64; i++ {
		payload := bytes.Repeat([]byte{byte(i)}, 1000)
		n, err := sender.WriteTo(payload, receiver.LocalAddr())
		if err != nil || n != len(payload) {
			t.Fatalf("Отправка: %d %v", n, err)
		}
		for j := range payload {
			payload[j] = 255
		} // вызывающий вправе сразу переиспользовать буфер
	}
	receiver.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 2000)
	for i := 0; i < 64; i++ {
		n, _, err := receiver.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1000 || !bytes.Equal(buf[:n], bytes.Repeat([]byte{byte(i)}, 1000)) {
			t.Fatal("Нарушен порядок либо повторно использован буфер")
		}
	}
	if err = sender.SetWriteDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err = sender.WriteTo([]byte("проверка"), receiver.LocalAddr()); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Не соблюдён срок отправки: %v", err)
	}
	if err = sender.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = sender.WriteTo([]byte("проверка"), receiver.LocalAddr()); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Отправка после закрытия: %v", err)
	}
}
