// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package blink

import (
	_ "embed"
	"math/rand/v2"
	"strings"
)

//go:embed chat-names.txt
var chatNamesFile string

var chatNames = strings.Split(strings.TrimSpace(chatNamesFile), "\n")

func RandomChatName() string {
	return chatNames[rand.IntN(len(chatNames))]
}
