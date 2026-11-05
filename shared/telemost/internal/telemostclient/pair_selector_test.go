// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPairHedgeSkipsBlockedFirstAndCleansUp(t *testing.T) {
	var alive, peak, commits atomic.Int32
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	index, active, err := selectPair(ctx, []int{0, 1, 2, 3}, 10*time.Millisecond, func(c context.Context, i int, commit func() bool) (bool, error) {
		n := alive.Add(1)
		defer alive.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		if i == 0 {
			<-c.Done()
			close(done)
			return false, nil
		}
		if i != 1 {
			t.Errorf("Лишний кандидат %d", i)
		}
		if !commit() {
			return false, nil
		}
		commits.Add(1)
		select {
		case <-done:
		case <-c.Done():
			return false, c.Err()
		}
		return true, nil
	}, nil)
	if err != nil || index != 1 || !active || commits.Load() != 1 || alive.Load() != 0 || peak.Load() != 2 {
		t.Fatalf("Результат: %d %v %v, активных %d, пик %d", index, active, err, alive.Load(), peak.Load())
	}
}
func TestPairSingleWinnerUnderRace(t *testing.T) {
	for k := 0; k < 40; k++ {
		var winners, alive atomic.Int32
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		i, active, err := selectPair(ctx, []int{0, 1, 2, 3, 4}, time.Nanosecond, func(c context.Context, i int, commit func() bool) (bool, error) {
			alive.Add(1)
			defer alive.Add(-1)
			if commit() {
				winners.Add(1)
				time.Sleep(time.Millisecond)
				return true, nil
			}
			return false, nil
		}, nil)
		cancel()
		if err != nil || i < 0 || !active || winners.Load() != 1 || alive.Load() != 0 {
			t.Fatalf("Гонка: %d %v %v победителей %d", i, active, err, winners.Load())
		}
	}
}
func TestPairRejectionsStayBounded(t *testing.T) {
	var alive, peak, attempts atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	i, active, err := selectPair(ctx, []int{0, 1, 2, 3, 4}, time.Nanosecond, func(c context.Context, i int, commit func() bool) (bool, error) {
		n := alive.Add(1)
		defer alive.Add(-1)
		attempts.Add(1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		if i == 4 && commit() {
			return true, nil
		}
		return false, nil
	}, nil)
	if err != nil || i != 4 || !active || peak.Load() > maxPairProbes || attempts.Load() != 5 || alive.Load() != 0 {
		t.Fatalf("Перебор: %d %v %v пик %d попыток %d", i, active, err, peak.Load(), attempts.Load())
	}
}
func TestPairCancellationWaitsForCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var alive atomic.Int32
	go func() { time.Sleep(10 * time.Millisecond); cancel() }()
	_, _, err := selectPair(ctx, []int{0, 1, 2}, time.Nanosecond, func(c context.Context, i int, commit func() bool) (bool, error) {
		alive.Add(1)
		defer alive.Add(-1)
		<-c.Done()
		time.Sleep(5 * time.Millisecond)
		return false, nil
	}, nil)
	if err != nil || alive.Load() != 0 {
		t.Fatal("Не завершилась очистка")
	}
}
func TestPairFailureIsPropagated(t *testing.T) {
	sentinel := errors.New("проверочная ошибка")
	_, _, err := selectPair(context.Background(), []int{0}, time.Second, func(context.Context, int, func() bool) (bool, error) { return false, sentinel }, nil)
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
func TestPairHistoryFollowsSubscription(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	links := []string{"первая", "вторая"}
	rememberPair(links, 1)
	if preferredPair(links, 3) != 1 || preferredPair([]string{"другая"}, 3) != -1 || preferredPair(links, 1) != -1 {
		t.Fatal("История не соответствует пулу")
	}
}

// Четыре занятые/медленные комнаты не должны блокировать пятую свободную на минуту.
func TestPairFifthFreeWhileFirstFourBlocked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var alive, commits atomic.Int32
	started := time.Now()
	index, active, err := selectPair(ctx, []int{0, 1, 2, 3, 4}, 10*time.Millisecond, func(c context.Context, i int, commit func() bool) (bool, error) {
		alive.Add(1)
		defer alive.Add(-1)
		if i < 4 {
			<-c.Done()
			return false, nil
		}
		if commit() {
			commits.Add(1)
			return true, nil
		}
		return false, nil
	}, nil)
	if err != nil || index != 4 || !active || commits.Load() != 1 || alive.Load() != 0 || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("Свободная пятая: %d %v %v; работающих %d", index, active, err, alive.Load())
	}
}
