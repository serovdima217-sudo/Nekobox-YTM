// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

//go:build linux

package main

import (
	"bufio"
	"os"
	"strings"
	"syscall"
)

func watchStdinQuit(sig chan<- os.Signal) {
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == "QUIT" {
				sig <- syscall.SIGTERM
				return
			}
		}
		sig <- syscall.SIGTERM
	}()
}
