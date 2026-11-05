// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package bond

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

type Mode byte

const (
	Speed Mode = iota
	Balanced
	Stability
	kindReceipt byte = 8
	kindPing    byte = 9
	kindPong    byte = 10
)

func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "скорость", "speed":
		return Speed, nil
	case "баланс", "balanced":
		return Balanced, nil
	case "устойчивость", "stability", "stable":
		return Stability, nil
	default:
		return Balanced, fmt.Errorf("режим должен быть: скорость, баланс или устойчивость")
	}
}
func (m Mode) String() string {
	switch m {
	case Speed:
		return "скорость"
	case Stability:
		return "устойчивость"
	default:
		return "баланс"
	}
}
func (m Mode) window() uint64 {
	switch m {
	case Speed:
		return 192
	case Stability:
		return 64
	default:
		return 128
	}
}
func (t *Transport) SetMode(m Mode) error {
	if m > Stability {
		return fmt.Errorf("неизвестный режим объединения")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.everReady {
		return fmt.Errorf("режим меняется только до установления сессии")
	}
	t.mode = m
	return nil
}

type sentSample struct {
	at   time.Time
	lane int
}

// Оценка учитывает доставку на другой конец, а не только помещение блока в очередь KCP.
func (t *Transport) chooseLaneLocked(size int) int {
	if t.selective && t.primaryDegraded && t.lanes[1].verified {
		if len(t.lanes[1].jobs) < cap(t.lanes[1].jobs) {
			return 1
		}
		return -1
	}
	chosen := -1
	best := math.Inf(1)
	width := t.width
	if t.paired {
		width = 2
	}
	if t.selective {
		width = 1
	}
	for n := 0; n < width; n++ {
		i := (t.cursor + n) % width
		l := &t.lanes[i]
		if !l.verified || len(l.jobs) >= cap(l.jobs) {
			continue
		}
		// Даже медленный канал проходит начальную проверку полезным трафиком.
		if l.sent == 0 {
			return i
		}
		rate := l.rate
		if rate <= 0 {
			rate = 256 * 1024
		} else if rate < 8192 {
			rate = 8192
		}
		rtt := l.rtt
		if rtt <= 0 {
			rtt = .3
		}
		flight := l.sent - l.delivered
		latencyWeight, jitterWeight := .5, 1.0
		if t.mode == Speed {
			latencyWeight = .2
			jitterWeight = .25
		} else if t.mode == Stability {
			latencyWeight = 1
			jitterWeight = 2
		}
		score := latencyWeight*rtt + jitterWeight*l.jitter + (float64(flight)+float64(size))/rate
		if score < best {
			best = score
			chosen = i
		}
	}
	return chosen
}
func (l *lane) sampleRTT(sample float64) {
	if sample <= 0 || sample > 120 {
		return
	}
	if l.rtt == 0 {
		l.rtt = sample
		return
	}
	delta := math.Abs(sample - l.rtt)
	l.jitter = .8*l.jitter + .2*delta
	l.rtt = .8*l.rtt + .2*sample
}
func (t *Transport) receiptLocked(i int, seq uint64, body []byte) {
	if len(body) != 8 {
		return
	}
	l := &t.lanes[i]
	total := binary.BigEndian.Uint64(body)
	if total <= l.delivered || total > l.sent {
		return
	}
	now := time.Now()
	delta := total - l.delivered
	if previous, ok := t.samples[seq]; ok && previous.lane == i {
		l.sampleRTT(now.Sub(previous.at).Seconds())
		delete(t.samples, seq)
	}
	// Короткие TLS/HTTP-запросы не измеряют ёмкость канала: трафика может просто не быть.
	if l.timeRate.IsZero() || (!l.lastReceipt.IsZero() && now.Sub(l.lastReceipt) > 500*time.Millisecond) {
		l.timeRate = now
		l.rateBytes = 0
	} else {
		l.rateBytes += delta
		if elapsed := now.Sub(l.timeRate).Seconds(); elapsed >= .25 && l.rateBytes >= 65536 {
			rate := float64(l.rateBytes) / elapsed
			if l.rate == 0 {
				l.rate = rate
			} else {
				l.rate = .85*l.rate + .15*rate
			}
			l.timeRate = now
			l.rateBytes = 0
		}
	}

	l.lastReceipt = now
	l.delivered = total
	notify(t.wake)
}
func (t *Transport) qualityProbes() {
	t.mu.Lock()
	if !t.ready || t.err != nil {
		t.mu.Unlock()
		return
	}
	t.pingCounter++
	packets := make([][]byte, t.width)
	for i := range t.lanes[:t.width] {
		l := &t.lanes[i]
		l.pingSeq = t.pingCounter
		l.pingAt = time.Now()
		packets[i] = t.encode(kindPing, i, t.pingCounter, nil)
	}
	t.mu.Unlock()
	for i, p := range packets {
		t.queueControl(i, p)
	}
}
