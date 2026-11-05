// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"testing"
	"time"
)

func TestЧасыДемонстрацииНеУскоряютсяСЧастотойПакетов(t *testing.T) {
	for _, rate := range []int{720, 2400, 7200} {
		var remainder int64
		var ticks uint32
		for n := 0; n < rate; n++ {
			ticks += sharingSamples(time.Second/time.Duration(rate), &remainder)
		}
		if ticks < 89990 || ticks > 90000 {
			t.Fatalf("неверная частота RTP при %d кадрах: %d", rate, ticks)
		}
	}
}
