// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package bond

import (
	"time"
)

const SelectiveLanes = 2
const kindGap byte = 13

// SelectiveProfile разделяет производительность основного и резервного носителей.
type SelectiveProfile struct {
	MainFPS      int  `json:"primary_fps"`
	MainBatch    int  `json:"primary_batch"`
	ReserveFPS   int  `json:"reserve_fps"`
	ReserveBatch int  `json:"reserve_batch"`
	Mode         Mode `json:"mode"`
}

var DefaultSelectiveMode = "баланс"

func DefaultSelectiveProfile() SelectiveProfile {
	mode, e := ParseMode(DefaultSelectiveMode)
	if e != nil {
		mode = Balanced
	}
	return SelectiveProfile{60, 120, 12, 40, mode}
}
func (p SelectiveProfile) Valid() bool {
	return p.MainFPS >= 1 && p.MainFPS <= 60 && p.MainBatch >= 1 && p.MainBatch <= 120 && p.ReserveFPS >= 1 && p.ReserveFPS < p.MainFPS && p.ReserveBatch >= 1 && p.ReserveBatch <= p.MainBatch && p.Mode <= Stability
}
func (p SelectiveProfile) Carrier(lane int) (int, int) {
	if lane == 0 {
		return p.MainFPS, p.MainBatch
	}
	return p.ReserveFPS, p.ReserveBatch
}

// NewSelective создаёт основной путь 0 и облегчённый путь восстановления 1.
// В отличие от 2+2, здоровый основной поток не копируется целиком.
func NewSelective(key [32]byte, server bool, log func(string, ...any)) *Transport {
	return newTransport(key, server, log, true, 0, SelectiveLanes)
}
func (t *Transport) selectiveRTOLocked() time.Duration {
	l := &t.lanes[0]
	rto := time.Duration((l.rtt + 4*l.jitter) * float64(time.Second))
	if rto < 900*time.Millisecond {
		rto = 900 * time.Millisecond
	}
	if rto > 5*time.Second {
		rto = 5 * time.Second
	}
	return rto
}

// Резерв не получает новые блоки просто из-за заполнения очереди основного.
// При скрытом отказе очищаются только уже подтверждённые копии из зависшей
// основной очереди; новые блоки восстанавливаются по тому же ограниченному кешу.
func (t *Transport) discardDeliveredLocked(lane int) {
	l := &t.lanes[lane]
	// Не извлекаем непроверенные элементы: параллельный писатель иначе мог бы
	// обогнать повторно помещённые блоки и создать искусственный пропуск.
	if t.ackedSeq < l.maxEnqueuedSeq {
		return
	}
	count := len(l.jobs)
	for n := 0; n < count; n++ {
		select {
		case p := <-l.jobs:
			l.queued -= len(p)
			notify(t.wake)
		default:
			return
		}
	}
}

// При получении блока за пропуском запрашивается именно первый недостающий.
// Запросы ограничены по частоте, чтобы потеря одного кадра не создала лавину копий.
func (t *Transport) requestGapLocked() {
	if _, ok := t.pending[t.recvSeq]; ok {
		return
	}
	now := time.Now()
	if t.lastGapSeq == t.recvSeq && now.Sub(t.lastGapAt) < 100*time.Millisecond {
		return
	}
	lane := 1
	if !t.lanes[lane].verified {
		lane = 0
	}
	if !t.lanes[lane].verified {
		return
	}
	t.queueControl(lane, t.encode(kindGap, lane, t.recvSeq, nil))
	t.lastGapSeq, t.lastGapAt = t.recvSeq, now
}
func (t *Transport) selectiveTickLocked(now time.Time) {
	if t.primaryDegraded && t.lanes[0].verified {
		l := &t.lanes[0]
		if l.pingAt.IsZero() || now.Sub(l.pingAt) > time.Second {
			t.pingCounter++
			l.pingSeq = t.pingCounter
			l.pingAt = now
			t.queueControl(0, t.encode(kindPing, 0, l.pingSeq, nil))
		}
	}
	for seq := range t.outbound {
		if seq <= t.ackedSeq {
			delete(t.outbound, seq)
			notify(t.wake)
		}
	}
	rto := t.selectiveRTOLocked()
	sent := 0
	for seq := t.ackedSeq + 1; seq < t.sendSeq; seq++ {
		f := t.outbound[seq]
		if f == nil {
			continue
		}
		if f.received && !f.requested {
			continue
		}
		age := now.Sub(f.lastRetry)
		if (!f.requested && age < rto) || (f.retries > 0 && age < 250*time.Millisecond) {
			continue
		}
		lane := 1
		if !t.lanes[lane].verified {
			lane = 0
		}
		if t.queueCopyLocked(lane, seq, f) {
			f.lastRetry = now
			f.retries++
			f.requested = false
			sent++
			// Здоровый основной не должен конкурировать с лавиной превентивных копий.
			if sent >= 2 && t.lanes[0].verified {
				break
			}
		}
	}
}

// TestBlackout доступен только вызывающему коду своего процесса. В обычном
// клиенте он не вызывается; Linux-испытатель включает его отдельным флагом.
// Это потеря пакетов одного пути, а не выключение по росту нагрузки.
func (t *Transport) TestBlackout(lane int, duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if lane < 0 || lane >= t.width || duration <= 0 {
		return
	}
	t.faultUntil[lane] = time.Now().Add(duration)
}

// Окно одной основной сессии не должно накапливать несколько мегабайт перед
// короткими TLS-запросами. Большие окна прежнего многополосного режима здесь
// создают задержку, а не дополнительную полезную скорость.
func (t *Transport) selectiveWindow() uint64 {
	switch t.mode {
	case Speed:
		return 64
	case Stability:
		return 16
	default:
		return 32
	}
}
