// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package common

import (
	"fmt"
	"os"
)

const Version = "0.4.4"

func MaybePrintVersion() {
	for _, arg := range os.Args[1:] {
		if arg == "-version" || arg == "--version" {
			fmt.Println(Version)
			os.Exit(0)
		}
	}
}
