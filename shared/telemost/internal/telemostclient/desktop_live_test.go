// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
	"whitelist-bypass/relay/bond"
)

// Только явный запуск испытателем; обычные go test не подключаются к Телемосту.
func TestDesktopLiveRecovery(t *testing.T) {
	file := os.Getenv("NEKOBOX_LIVE_CONFIG")
	if file == "" {
		t.Skip("Натурная проверка включается отдельно")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var c ClientConfig
	if json.Unmarshal(data, &c) != nil {
		t.Fatal("Неверная тестовая конфигурация")
	}
	if err = configureDNS(""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	done := make(chan error, 1)
	ready := make(chan *bond.Transport, 8)
	go func() {
		done <- RunSelectiveLinksDiagnostics(ctx, "127.0.0.1:29998", bond.DefaultSelectiveProfile(), c.Links, func(tr *bond.Transport) {
			select {
			case ready <- tr:
			default:
			}
		})
	}()
	defer func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(12 * time.Second):
			t.Error("Транспорт не завершил очистку")
		}
	}()
	start := time.Now()
	var tr *bond.Transport
	select {
	case tr = <-ready:
	case <-ctx.Done():
		t.Fatal("Пара не подготовилась")
	}
	t.Logf("Пара готова за %.1f с", time.Since(start).Seconds())
	var conn net.Conn
	for until := time.Now().Add(5 * time.Second); time.Now().Before(until); {
		conn, err = net.DialTimeout("tcp", "127.0.0.1:29998", time.Second)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(45 * time.Second))
	if _, err = conn.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	var hello [2]byte
	if _, err = io.ReadFull(conn, hello[:]); err != nil || hello != [2]byte{5, 0} {
		t.Fatalf("SOCKS: %v", err)
	}
	host := "example.com"
	request := append([]byte{5, 1, 0, 3, byte(len(host))}, []byte(host)...)
	request = binary.BigEndian.AppendUint16(request, 443)
	if _, err = conn.Write(request); err != nil {
		t.Fatal(err)
	}
	var response [4]byte
	if _, err = io.ReadFull(conn, response[:]); err != nil || response[1] != 0 {
		t.Fatalf("CONNECT: %v код %d", err, response[1])
	}
	size := 0
	switch response[3] {
	case 1:
		size = 4
	case 4:
		size = 16
	case 3:
		var n [1]byte
		io.ReadFull(conn, n[:])
		size = int(n[0])
	default:
		t.Fatal("Неверный адрес SOCKS")
	}
	if _, err = io.CopyN(io.Discard, conn, int64(size+2)); err != nil {
		t.Fatal(err)
	}
	stream := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err = stream.HandshakeContext(ctx); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(stream)
	for i := 0; i < 12; i++ {
		if i == 3 {
			tr.TestBlackout(0, 10*time.Second)
			t.Log("Отдельный тест: потеря пакетов основного пути на 10 с")
		}
		if i == 7 {
			tr.TestBlackout(1, 10*time.Second)
			t.Log("Отдельный тест: потеря пакетов резерва на 10 с")
		}
		stream.SetDeadline(time.Now().Add(45 * time.Second))
		_, err = fmt.Fprintf(stream, "GET / HTTP/1.1\r\nHost: example.com\r\nUser-Agent: NekoboxYTM-test\r\nConnection: keep-alive\r\n\r\n")
		if err != nil {
			t.Fatal(err)
		}
		r, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
		if err != nil {
			t.Fatal(err)
		}
		n, err := io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if err != nil || r.StatusCode != 200 || n < 100 {
			t.Fatalf("HTTPS %d, %d байт: %v", r.StatusCode, n, err)
		}
		t.Logf("Один и тот же TCP/TLS, HTTPS %d/12: 200, %d байт", i+1, n)
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Second):
		}
	}
	if tr.Err() != nil {
		t.Fatal(tr.Err())
	}
	t.Log("Сквозной TCP/TLS сохранился при отдельных потерях основного и резервного путей")
}
