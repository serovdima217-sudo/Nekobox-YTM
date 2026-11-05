// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package roomlease

import (
	"crypto/sha256"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestОдновременнаяБроньНеДелитЧетвёрку(t *testing.T) {
	b, _ := New(5, time.Minute)
	var wg sync.WaitGroup
	var wins atomic.Int32
	start := make(chan struct{})
	for i := 1; i <= 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			var id Owner
			id[0] = byte(i)
			if ok, fresh := b.Acquire(0, id); ok && fresh {
				wins.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("выбрано несколько владельцев одной четвёрки")
	}
	owner, ok := b.Owner(0)
	if !ok || owner == (Owner{}) {
		t.Fatal("бронь потеряна")
	}
	for group := 1; group < 5; group++ {
		var id Owner
		id[0] = byte(200 + group)
		if ok, _ := b.Acquire(group, id); !ok {
			t.Fatal("посторонняя группа занята")
		}
	}
}
func TestПродлениеИстечениеИЗащитаОтСтарогоВладельца(t *testing.T) {
	b, _ := New(1, time.Minute)
	var clock atomic.Int64
	b.now = func() time.Time { return time.Unix(0, clock.Load()) }
	a, c := Owner{1}, Owner{2}
	if ok, _ := b.Acquire(0, a); !ok {
		t.Fatal("нет начальной брони")
	}
	clock.Store(int64(30 * time.Second))
	if !b.Touch(0, a) {
		t.Fatal("владелец не продлил бронь")
	}
	clock.Store(int64(65 * time.Second))
	if ok, _ := b.Acquire(0, c); ok {
		t.Fatal("продление проигнорировано")
	}
	clock.Store(int64(91 * time.Second))
	if b.Touch(0, a) {
		t.Fatal("просроченная бронь ожила")
	}
	if ok, fresh := b.Acquire(0, c); !ok || !fresh {
		t.Fatal("нет повторной выдачи")
	}
	if b.Release(0, a) || b.Touch(0, a) {
		t.Fatal("старый владелец изменил чужую бронь")
	}
	if !b.Release(0, c) {
		t.Fatal("владелец не освободил бронь")
	}
	if _, ok := b.Owner(0); ok {
		t.Fatal("бронь не освобождена")
	}
}
func TestПовторныеЗаявкиНеПродлеваютБроньБезМедиаканала(t *testing.T) {
	b, _ := New(1, time.Minute)
	var clock atomic.Int64
	b.now = func() time.Time { return time.Unix(0, clock.Load()) }
	a := Owner{1}
	b.Acquire(0, a)
	clock.Store(int64(59 * time.Second))
	b.Acquire(0, a)
	clock.Store(int64(61 * time.Second))
	if _, ok := b.Owner(0); ok {
		t.Fatal("бронь продлена одним опросом зала ожидания")
	}
}
func TestПодписьЗаявкиПривязанаКЧетвёркеИВладельцу(t *testing.T) {
	key := sha256.Sum256([]byte("одна четвёрка"))
	other := sha256.Sum256([]byte("другая четвёрка"))
	owner := Owner{1, 2, 3}
	name := Name(key, owner)
	if len(name) != 60 {
		t.Fatal("имя превышает согласованный размер")
	}
	if got, ok := ParseName(key, name); !ok || got != owner {
		t.Fatal("заявка не проверена")
	}
	if _, ok := ParseName(other, name); ok {
		t.Fatal("принят ключ другой группы")
	}
	for _, bad := range []string{name + "x", name[:len(name)-1], name[:4] + "f" + name[5:], Name(key, Owner{}), "Гость"} {
		if _, ok := ParseName(key, bad); ok {
			t.Fatal("подмена имени принята")
		}
	}
}
