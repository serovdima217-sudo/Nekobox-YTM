// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package bond

import (
	"context"
	"fmt"
	"sort"
	"time"
)

const (
	PairedLanes       = 4
	kindRelease  byte = 11
	kindReleased byte = 12
)

// pairedFrame хранится до сквозного подтверждения и отправки задержанной копии.
// Один ограниченный буфер заменяет отдельные таймеры и горутины для каждого блока.
type pairedFrame struct {
	body                 []byte
	firstLane            int
	copyDue, lastRetry   time.Time
	copyQueued, received bool
	requested            bool
	retries              int
}

// NewPaired создаёт две основные сессии 0/1 и парные резервные 2/3.
// Иной идентификатор протокола исключает смешивание со старым распределением 4 полос.
func NewPaired(key [32]byte, server bool, log func(string, ...any), delay time.Duration) *Transport {
	if delay < 0 || delay > 2*time.Second {
		panic("неверная задержка резервных копий")
	}
	return newTransport(key, server, log, true, delay, PairedLanes)
}

func (t *Transport) magic() string {
	if t.selective {
		return "TMS4"
	}
	if t.paired {
		return "TMP2"
	}
	return "TMB1"
}

// ClientIdentifier используется для бронирования сразу всей четвёрки на сервере.
func (t *Transport) ClientIdentifier() [16]byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.clientID
}

// BindClient разрешён до рукопожатия: бронь из служебного канала привязывается
// к тому же идентификатору, который затем подтверждается в каждой медиасессии.
func (t *Transport) BindClient(id [16]byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.server || !t.paired || id == ([16]byte{}) {
		return fmt.Errorf("неверная привязка владельца четвёрки")
	}
	if t.clientID != ([16]byte{}) && t.clientID != id {
		return fmt.Errorf("четвёрка занята другим владельцем")
	}
	t.clientID = id
	return nil
}

func (t *Transport) chooseReserveLocked() int {
	if t.selective {
		if t.lanes[0].verified && !t.primaryDegraded {
			t.discardDeliveredLocked(0)
			return -1
		}
		if t.lanes[1].verified && len(t.lanes[1].jobs) < cap(t.lanes[1].jobs) {
			return 1
		}
		return -1
	}
	for n := 0; n < 2; n++ {
		i := 2 + (t.cursor+n)%2
		if t.lanes[i].verified && len(t.lanes[i].jobs) < cap(t.lanes[i].jobs) {
			return i
		}
	}
	return -1
}

func (t *Transport) queueCopyLocked(i int, seq uint64, f *pairedFrame) bool {
	l := &t.lanes[i]
	if !l.verified || l.raw == nil {
		return false
	}
	p := t.encode(kindData, i, seq, f.body)
	select {
	case l.jobs <- p:
		l.queued += len(p)
		if seq > l.maxEnqueuedSeq {
			l.maxEnqueuedSeq = seq
		}
		l.sent += uint64(len(f.body))
		return true
	default:
		return false
	}
}

// redundancy постоянно обслуживает отложенные копии, в том числе при успешной
// доставке основной. Пропуски, не дошедшие по обоим парным путям, повторяются
// по оставшимся сессиям без смены эпохи, RelayBridge и TCP-соединения.
func (t *Transport) redundancy() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.stop:
			return
		case now := <-ticker.C:
			t.mu.Lock()
			if t.err != nil {
				t.mu.Unlock()
				return
			}
			if t.selective {
				t.selectiveTickLocked(now)
				t.mu.Unlock()
				continue
			}
			// Порядок обхода важен: сначала восстанавливается самый ранний пропуск.
			seqs := make([]uint64, 0, len(t.outbound))
			for seq := range t.outbound {
				seqs = append(seqs, seq)
			}
			sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
			for _, seq := range seqs {
				f := t.outbound[seq]
				if f == nil {
					continue
				}
				other := (f.firstLane + 2) % PairedLanes
				if !f.copyQueued && !now.Before(f.copyDue) {
					f.copyQueued = t.queueCopyLocked(other, seq, f)
				}
				if seq <= t.ackedSeq {
					// При недоступном резерве уже доставленные байты хранить незачем.
					if f.copyQueued || (!now.Before(f.copyDue) && (!t.lanes[other].verified || now.Sub(f.copyDue) > 2*time.Second)) {
						delete(t.outbound, seq)
						notify(t.wake)
					}
					continue
				}
				if f.received {
					continue
				}
				retryAfter := 2 * time.Second
				for i := range t.lanes[:PairedLanes] {
					l := &t.lanes[i]
					if l.verified && l.rtt > 0 {
						v := time.Duration((2*l.rtt + .5) * float64(time.Second))
						if v > retryAfter {
							retryAfter = v
						}
					}
				}
				if retryAfter > 15*time.Second {
					retryAfter = 15 * time.Second
				}
				if now.Sub(f.lastRetry) < retryAfter {
					continue
				}
				for n := 0; n < PairedLanes; n++ {
					i := (f.firstLane + 1 + f.retries + n) % PairedLanes
					if t.queueCopyLocked(i, seq, f) {
						f.lastRetry = now
						f.retries++
						break
					}
				}
			}
			t.mu.Unlock()
		}
	}
}

// Обработчики серверного учёта вызываются вне блокировки транспорта.
func (t *Transport) SetLeaseCallbacks(activity, release func([16]byte)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onPeerActivity = activity
	t.onRelease = release
}

// ReleaseLease запрашивает освобождение четвёрки и ограниченно ждёт подтверждение.
// Если сеть уже пропала, сервер освободит бронь по сроку, а не по догадке клиента.
func (t *Transport) ReleaseLease(ctx context.Context) bool {
	t.mu.Lock()
	if !t.paired || t.server || t.err != nil {
		t.mu.Unlock()
		return false
	}
	packets := make(map[int][]byte)
	for i := 0; i < t.width; i++ {
		if t.lanes[i].verified {
			packets[i] = t.encode(kindRelease, i, 0, nil)
		}
	}
	t.mu.Unlock()
	for i, p := range packets {
		t.queueControl(i, p)
	}
	select {
	case <-t.released:
		return true
	case <-ctx.Done():
		return false
	case <-t.stop:
		return false
	}
}
