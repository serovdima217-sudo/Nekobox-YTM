// SPDX-License-Identifier: GPL-3.0-or-later
// Контролируемый тест задержки, без сети, приглашений и настоящих комнат.
package telemostclient

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestPairLatencyWithProductionHedge(t *testing.T) {
	const blocked = 2 * time.Second
	const ready = 40 * time.Millisecond
	const hedge = 600 * time.Millisecond
	sequentialStart := time.Now()
	time.Sleep(blocked)
	time.Sleep(ready)
	sequential := time.Since(sequentialStart)
	var commits, alive atomic.Int32
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	index, active, err := selectPair(ctx, []int{0, 1}, hedge, func(c context.Context, i int, commit func() bool) (bool, error) {
		alive.Add(1)
		defer alive.Add(-1)
		wait := ready
		if i == 0 {
			wait = blocked
		}
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-c.Done():
			return false, nil
		case <-timer.C:
		}
		if i == 0 {
			return false, nil
		}
		if !commit() {
			return false, nil
		}
		commits.Add(1)
		return true, nil
	}, nil)
	elapsed := time.Since(start)
	if err != nil || index != 1 || !active || commits.Load() != 1 || alive.Load() != 0 {
		t.Fatalf("Неверный выбор/очистка: индекс=%d, активен=%v, ошибка=%v, победителей=%d, оставшихся=%d", index, active, err, commits.Load(), alive.Load())
	}
	if elapsed >= sequential*3/4 {
		t.Fatalf("Нет ожидаемого выигрыша на контролируемом сценарии: последовательно=%s, параллельно=%s", sequential, elapsed)
	}
	t.Logf("Локальный сценарий, не замер Телемоста: блокировка=%s, свободная=%s, stagger=%s; последовательно=%s, с опережающей пробой=%s; победитель один, проигравшие завершены", blocked, ready, hedge, sequential, elapsed)
}
