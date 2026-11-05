// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

// Package dup передаёт немедленную и отложенную копии блока с ограниченной очередью.
package dup

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"
)

const HeaderSize = 20
const windowSize = 8192

var magic = [4]byte{0xd3, 0x75, 0x70, 1}

type queued struct {
	data []byte
	at   time.Time
}
type replay struct {
	epoch uint64
	top   uint64
	seen  [windowSize]uint64
}
type Stats struct {
	Primary, Secondary, QueueDrops, Duplicates, Recovered uint64
	QueuedBytes                                           int
}

// Carrier не ожидает освобождения очередей обоих путей и не создаёт горутину на пакет.
type Carrier struct {
	mu                                                         sync.Mutex
	sendMu                                                     sync.Mutex
	delay                                                      time.Duration
	primary, secondary                                         func([]byte) bool
	epoch, next                                                uint64
	queue                                                      []queued
	head, count, bytes, maxBytes                               int
	wake, done                                                 chan struct{}
	wg                                                         sync.WaitGroup
	once                                                       sync.Once
	closed                                                     bool
	receiveMu                                                  sync.Mutex
	histories                                                  [2]replay
	current                                                    int
	primaryCount, secondaryCount, drops, duplicates, recovered atomic.Uint64
}

func New(delay time.Duration, capacity, maxBytes int, p, s func([]byte) bool) *Carrier {
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	if delay > 2*time.Second {
		delay = 2 * time.Second
	}
	if capacity < 1 {
		capacity = 1
	}
	if maxBytes < HeaderSize {
		maxBytes = HeaderSize
	}
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		panic("не удалось получить случайный идентификатор потока")
	}
	c := &Carrier{delay: delay, primary: p, secondary: s, epoch: binary.BigEndian.Uint64(seed[:]), next: 1, queue: make([]queued, capacity), maxBytes: maxBytes, wake: make(chan struct{}, 1), done: make(chan struct{})}
	if c.epoch == 0 {
		c.epoch = 1
	}
	c.wg.Add(1)
	go c.loop()
	return c
}
func (c *Carrier) Send(data []byte) bool {
	if len(data) > c.maxBytes-HeaderSize {
		return false
	}
	// Порядок постановки совпадает с порядком номеров и сроков, даже при конкурирующих отправителях.
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	seq := c.next
	c.next++
	c.mu.Unlock()
	wire := make([]byte, HeaderSize+len(data))
	copy(wire, magic[:])
	binary.BigEndian.PutUint64(wire[4:12], c.epoch)
	binary.BigEndian.PutUint64(wire[12:20], seq)
	copy(wire[20:], data)
	ok := c.primary(wire)
	if ok {
		c.primaryCount.Add(1)
	}
	c.mu.Lock()
	if !c.closed && c.count < len(c.queue) && c.bytes+len(wire) <= c.maxBytes {
		idx := (c.head + c.count) % len(c.queue)
		c.queue[idx] = queued{wire, time.Now().Add(c.delay)}
		c.count++
		c.bytes += len(wire)
	} else {
		c.drops.Add(1)
	}
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
	// Даже если камера отказала, принятая отложенная копия может восстановить блок.
	return ok
}
func (c *Carrier) loop() {
	defer c.wg.Done()
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		if c.count == 0 {
			c.mu.Unlock()
			select {
			case <-c.done:
				return
			case <-c.wake:
			}
			continue
		}
		q := c.queue[c.head]
		wait := time.Until(q.at)
		if wait > 0 {
			c.mu.Unlock()
			timer.Reset(wait)
			select {
			case <-c.done:
				return
			case <-c.wake:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			case <-timer.C:
			}
			continue
		}
		c.queue[c.head] = queued{}
		c.head = (c.head + 1) % len(c.queue)
		c.count--
		c.bytes -= len(q.data)
		c.mu.Unlock()
		if c.secondary(q.data) {
			c.secondaryCount.Add(1)
		}
	}
}

// Receive возвращает только первую принятую копию. fromSecondary нужен только для статистики.
func (c *Carrier) Receive(wire []byte, fromSecondary bool) ([]byte, bool) {
	if len(wire) < HeaderSize || string(wire[:4]) != string(magic[:]) {
		return nil, false
	}
	epoch, seq := binary.BigEndian.Uint64(wire[4:12]), binary.BigEndian.Uint64(wire[12:20])
	if epoch == 0 || seq == 0 {
		return nil, false
	}
	c.receiveMu.Lock()
	defer c.receiveMu.Unlock()
	idx := -1
	for i := range c.histories {
		if c.histories[i].epoch == epoch {
			idx = i
			break
		}
	}
	if idx < 0 {
		c.current = (c.current + 1) % len(c.histories)
		idx = c.current
		c.histories[idx] = replay{epoch: epoch}
	}
	h := &c.histories[idx]
	if h.top >= windowSize && seq <= h.top-windowSize {
		c.duplicates.Add(1)
		return nil, false
	}
	if seq > h.top {
		h.top = seq
	}
	pos := seq % windowSize
	if h.seen[pos] == seq {
		c.duplicates.Add(1)
		return nil, false
	}
	h.seen[pos] = seq
	if fromSecondary {
		c.recovered.Add(1)
	}
	return wire[HeaderSize:], true
}
func (c *Carrier) Stats() Stats {
	c.mu.Lock()
	n := c.bytes
	c.mu.Unlock()
	return Stats{c.primaryCount.Load(), c.secondaryCount.Load(), c.drops.Load(), c.duplicates.Load(), c.recovered.Load(), n}
}
func (c *Carrier) Close() {
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		for i := range c.queue {
			c.queue[i] = queued{}
		}
		c.bytes = 0
		c.count = 0
		close(c.done)
		c.mu.Unlock()
	})
	c.wg.Wait()
}

func HasEnvelope(wire []byte) bool {
	return len(wire) >= HeaderSize && string(wire[:4]) == string(magic[:])
}
