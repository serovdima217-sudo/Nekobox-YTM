// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

//go:build !linux

package main

import "fmt"

func lockManaged(path string) (func(), error) {
	return nil, fmt.Errorf("многокомнатный сервер в этой сборке поддерживается только на Linux")
}
