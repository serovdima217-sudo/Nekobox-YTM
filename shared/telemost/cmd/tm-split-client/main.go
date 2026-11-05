// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"whitelist-bypass/internal/telemostclient"
)

func main() {
	path := flag.String("config", "", "файл конфигурации TM Split")
	flag.Parse()
	if *path == "" {
		log.Fatal("Не указан файл конфигурации TM Split")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := telemostclient.RunConfigured(ctx, *path); err != nil {
		log.Fatal(err)
	}
}
