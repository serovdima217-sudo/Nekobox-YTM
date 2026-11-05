// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Проверяются только основные комнаты. Резерв и SOCKS запускает единственный
// победитель; проигравший освобождает бронь до завершения всей проверки.
// Одновременно не более пяти проб основных комнат; резерв запускает только победитель.
const maxPairProbes = 5

type pairAttempt func(context.Context, int, func() bool) (bool, error)

func selectPair(ctx context.Context, order []int, hedge time.Duration, attempt pairAttempt, chosen func(int)) (int, bool, error) {
	if len(order) == 0 {
		return -1, false, nil
	}
	type result struct {
		index  int
		active bool
		err    error
	}
	results := make(chan result, len(order))
	won := make(chan struct{})
	var mu sync.Mutex
	var workers sync.WaitGroup
	cancels := map[int]context.CancelFunc{}
	winner := -1
	defer func() {
		mu.Lock()
		for _, cancel := range cancels {
			cancel()
		}
		mu.Unlock()
		workers.Wait()
	}()
	launch := func(index int) bool {
		child, cancel := context.WithCancel(ctx)
		mu.Lock()
		if winner >= 0 || ctx.Err() != nil {
			mu.Unlock()
			cancel()
			return false
		}
		cancels[index] = cancel
		mu.Unlock()
		workers.Add(1)
		go func() {
			defer workers.Done()
			commit := func() bool {
				mu.Lock()
				defer mu.Unlock()
				if child.Err() != nil || winner >= 0 {
					return false
				}
				winner = index
				for other, stop := range cancels {
					if other != index {
						stop()
					}
				}
				if chosen != nil {
					chosen(index)
				}
				close(won)
				return true
			}
			active, err := attempt(child, index, commit)
			results <- result{index, active, err}
		}()
		return true
	}
	launch(order[0])
	next, inflight := 1, 1
	timer := time.NewTimer(hedge)
	defer timer.Stop()
	limit := 1
	for inflight > 0 {
		select {
		case <-ctx.Done():
			return -1, false, nil
		case <-won:
			// После выбора новые комнаты больше не открываются.
			won = nil
		case <-timer.C:
			if limit < maxPairProbes {
				limit++
			}
			timer.Reset(hedge)
		case r := <-results:
			inflight--
			mu.Lock()
			picked := winner
			mu.Unlock()
			if picked == r.index {
				return picked, r.active, r.err
			}
			if r.err != nil && picked < 0 {
				return -1, false, r.err
			}
		}
		mu.Lock()
		picked := winner
		mu.Unlock()
		for picked < 0 && next < len(order) && inflight < limit {
			if !launch(order[next]) {
				break
			}
			next++
			inflight++
		}
	}
	return -1, false, nil
}

// В кэше только хеш пула и номер последней удачной пары, без ссылок и ключей.
// Ошибка записи не мешает подключению; изменение подписки обнуляет подсказку.
type pairHistory struct {
	Pool  string `json:"pool"`
	Index int    `json:"index"`
}

func pairHistoryPath() string {
	root, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(root, "nekobox-ytm", "last-pair.json")
}
func poolFingerprint(links []string) string {
	h := sha256.New()
	for _, s := range links {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
func preferredPair(links []string, n int) int {
	b, err := os.ReadFile(pairHistoryPath())
	if err != nil {
		return -1
	}
	var v pairHistory
	if json.Unmarshal(b, &v) != nil || v.Pool != poolFingerprint(links) || v.Index < 0 || v.Index >= n {
		return -1
	}
	return v.Index
}
func rememberPair(links []string, index int) {
	path := pairHistoryPath()
	if path == "" {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return
	}
	b, _ := json.Marshal(pairHistory{poolFingerprint(links), index})
	f, err := os.CreateTemp(filepath.Dir(path), ".last-pair-*")
	if err != nil {
		return
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return
	}
	if f.Close() != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
