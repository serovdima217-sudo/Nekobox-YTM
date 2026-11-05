// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

// Package roomlease резервирует целые четвёрки через один серверный учёт.
// Доставка заявки идёт через зал ожидания Телемоста, прямое соединение с VPS не нужно.
package roomlease

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Owner [16]byte
type entry struct {
	owner Owner
	until time.Time
}
type Book struct {
	mu      sync.Mutex
	entries []entry
	ttl     time.Duration
	now     func() time.Time
}

func New(groups int, ttl time.Duration) (*Book, error) {
	if groups < 1 || ttl <= 0 {
		return nil, fmt.Errorf("неверный размер пула или срок брони")
	}
	return &Book{entries: make([]entry, groups), ttl: ttl, now: time.Now}, nil
}

// Acquire меняет владельца всей четвёрки одной атомарной операцией. Повторный
// запрос того же владельца не продлевает срок: его продлевает только медиаканал.
func (b *Book) Acquire(group int, owner Owner) (ok, fresh bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if group < 0 || group >= len(b.entries) || owner == (Owner{}) {
		return false, false
	}
	e := &b.entries[group]
	now := b.now()
	if e.owner != (Owner{}) && now.Before(e.until) {
		return e.owner == owner, false
	}
	*e = entry{owner: owner, until: now.Add(b.ttl)}
	return true, true
}
func (b *Book) Owner(group int) (Owner, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if group < 0 || group >= len(b.entries) {
		return Owner{}, false
	}
	e := b.entries[group]
	if e.owner == (Owner{}) || !b.now().Before(e.until) {
		return Owner{}, false
	}
	return e.owner, true
}
func (b *Book) Touch(group int, owner Owner) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if group < 0 || group >= len(b.entries) {
		return false
	}
	e := &b.entries[group]
	now := b.now()
	if e.owner != owner || owner == (Owner{}) || !now.Before(e.until) {
		return false
	}
	e.until = now.Add(b.ttl)
	return true
}
func (b *Book) Release(group int, owner Owner) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if group < 0 || group >= len(b.entries) || owner == (Owner{}) || b.entries[group].owner != owner {
		return false
	}
	b.entries[group] = entry{}
	return true
}

// Заявка помещается в имя гостя, доступное серверу в зале ожидания до допуска.
// 60 ASCII-символов: случайный идентификатор 128 бит и HMAC-подпись 96 бит.
// Знание всех четырёх приглашений подтверждается подписью ключом их группы.
func Name(key [32]byte, owner Owner) string {
	id := hex.EncodeToString(owner[:])
	m := hmac.New(sha256.New, key[:])
	m.Write([]byte("четвёрка-v1:" + id))
	return "T4." + id + "." + hex.EncodeToString(m.Sum(nil)[:12])
}
func ParseName(key [32]byte, name string) (Owner, bool) {
	var owner Owner
	parts := strings.Split(name, ".")
	if len(parts) != 3 || parts[0] != "T4" || len(parts[1]) != 32 || len(parts[2]) != 24 {
		return owner, false
	}
	id, e := hex.DecodeString(parts[1])
	if e != nil {
		return owner, false
	}
	copy(owner[:], id)
	if owner == (Owner{}) || !hmac.Equal([]byte(Name(key, owner)), []byte(name)) {
		return Owner{}, false
	}
	return owner, true
}

// NameForWidth отделяет заявки новой пары от прежних четвёрок.
func NameForWidth(key [32]byte, owner Owner, width int) string {
	if width == 4 {
		return Name(key, owner)
	}
	if width != 2 {
		return ""
	}
	id := hex.EncodeToString(owner[:])
	m := hmac.New(sha256.New, key[:])
	m.Write([]byte("пара-v1:" + id))
	return "T2." + id + "." + hex.EncodeToString(m.Sum(nil)[:12])
}
func ParseNameForWidth(key [32]byte, name string, width int) (Owner, bool) {
	if width == 4 {
		return ParseName(key, name)
	}
	var owner Owner
	if width != 2 || len(name) != 60 || !strings.HasPrefix(name, "T2.") {
		return owner, false
	}
	id, e := hex.DecodeString(name[3:35])
	if e != nil {
		return owner, false
	}
	copy(owner[:], id)
	if owner == (Owner{}) || !hmac.Equal([]byte(NameForWidth(key, owner, width)), []byte(name)) {
		return Owner{}, false
	}
	return owner, true
}
