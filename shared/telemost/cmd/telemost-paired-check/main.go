// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
	"whitelist-bypass/internal/telemostclient"
	"whitelist-bypass/relay/bond"
)

// Только испытательная Linux-сборка. Мобильный клиент не читает флаги.
func main() {
	port := flag.Int("socks-port", 9999, "локальный порт для одновременного испытания нескольких клиентов")
	primaryFPS := flag.Int("primary-fps", 60, "частота основного носителя")
	primaryBatch := flag.Int("primary-batch", 120, "интенсивность основного носителя")
	reserveFPS := flag.Int("reserve-fps", 12, "частота резервного носителя")
	reserveBatch := flag.Int("reserve-batch", 40, "интенсивность резервного носителя")
	modeArg := flag.String("bond-mode", "баланс", "режим окна передачи")
	faultAfter := flag.Duration("fault-after", 0, "только испытание: задержка до потери основного пути")
	faultFor := flag.Duration("fault-for", 8*time.Second, "только испытание: длительность потери основного пути")
	flag.Parse()
	if *port < 1024 || *port > 65535 {
		log.Fatal("неверный локальный порт")
	}
	debug.SetMemoryLimit(128 << 20)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	mode, e := bond.ParseMode(*modeArg)
	if e != nil {
		log.Fatal(e)
	}
	profile := bond.SelectiveProfile{MainFPS: *primaryFPS, MainBatch: *primaryBatch, ReserveFPS: *reserveFPS, ReserveBatch: *reserveBatch, Mode: mode}
	var hook func(*bond.Transport)
	if *faultAfter > 0 {
		hook = func(t *bond.Transport) {
			go func() {
				timer := time.NewTimer(*faultAfter)
				defer timer.Stop()
				select {
				case <-t.Done():
					return
				case <-ctx.Done():
					return
				case <-timer.C:
				}
				log.Printf("ИСПЫТАТЕЛЬНАЯ ПОТЕРЯ ОСНОВНОГО: %s", *faultFor)
				t.TestBlackout(0, *faultFor)
			}()
		}
	}
	if e := telemostclient.RunSelectiveDiagnostics(ctx, fmt.Sprintf("127.0.0.1:%d", *port), profile, hook); e != nil {
		log.Fatal(e)
	}
}
