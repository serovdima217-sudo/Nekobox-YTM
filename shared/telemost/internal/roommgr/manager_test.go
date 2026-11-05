// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package roommgr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	mu            sync.Mutex
	creates, runs map[int]int
	active, peak  int
	started       chan int
	fail          chan int
	state         string
	violation     bool
}

func fake(state string) *fakeBackend {
	return &fakeBackend{creates: map[int]int{}, runs: map[int]int{}, started: make(chan int, 100), fail: make(chan int, 10), state: state}
}
func (b *fakeBackend) Create(ctx context.Context, slot int) (string, error) {
	b.mu.Lock()
	b.creates[slot]++
	b.mu.Unlock()
	return fmt.Sprintf("https://telemost.yandex.ru/j/10000%d", slot), nil
}
func (b *fakeBackend) Run(ctx context.Context, slot int, link string) error {
	raw, e := os.ReadFile(b.state)
	var rooms []Room
	json.Unmarshal(raw, &rooms)
	saved := false
	for _, r := range rooms {
		if r.Slot == slot && r.Link == link {
			saved = true
		}
	}
	b.mu.Lock()
	b.violation = b.violation || e != nil || !saved
	b.runs[slot]++
	b.active++
	if b.active > b.peak {
		b.peak = b.active
	}
	b.mu.Unlock()
	defer func() { b.mu.Lock(); b.active--; b.mu.Unlock() }()
	b.started <- slot
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case n := <-b.fail:
			if n == slot {
				return fmt.Errorf("имитация разрыва")
			}
			b.fail <- n
			time.Sleep(time.Millisecond)
		}
	}
}
func options(dir string) Options {
	return Options{Sessions: 5, AutoCreate: true, StateFile: filepath.Join(dir, "rooms.json"), LinksFile: filepath.Join(dir, "links.txt"), RetryDelay: 5 * time.Millisecond, CreateDelay: time.Millisecond}
}
func collect(t *testing.T, b *fakeBackend) {
	t.Helper()
	seen := map[int]bool{}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for len(seen) < 5 {
		select {
		case n := <-b.started:
			seen[n] = true
		case <-timer.C:
			t.Fatal("не запущены все пять комнат")
		}
	}
}
func start(t *testing.T, m *Manager) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	return cancel, done
}
func stop(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("менеджер не остановился")
	}
}

func TestПятьНезависимыхСессий(t *testing.T) {
	o := options(t.TempDir())
	b := fake(o.StateFile)
	m, e := New(o, b)
	if e != nil {
		t.Fatal(e)
	}
	cancel, done := start(t, m)
	defer cancel()
	collect(t, b)
	b.mu.Lock()
	peak, bad := b.peak, b.violation
	b.mu.Unlock()
	if peak != 5 || bad {
		t.Fatal("нет пяти одновременных сессий либо комната запущена до сохранения")
	}
	b.fail <- 3
	select {
	case n := <-b.started:
		if n != 3 {
			t.Fatal("перезапущена чужая комната")
		}
	case <-time.After(time.Second):
		t.Fatal("комната не переподключилась")
	}
	b.mu.Lock()
	for slot, n := range b.runs {
		if slot != 3 && n != 1 {
			t.Error("разрыв перезапустил другую комнату")
		}
	}
	b.mu.Unlock()
	stop(t, cancel, done)
	b.mu.Lock()
	active := b.active
	for _, n := range b.creates {
		if n != 1 {
			t.Error("создана лишняя комната")
		}
	}
	b.mu.Unlock()
	if active != 0 {
		t.Fatal("сессии не остановлены")
	}
	info, _ := os.Stat(o.StateFile)
	if info.Mode().Perm() != 0600 {
		t.Fatal("небезопасные права состояния")
	}
	// После перезапуска не создаём новые ссылки.
	b2 := fake(o.StateFile)
	m2, e := New(o, b2)
	if e != nil {
		t.Fatal(e)
	}
	cancel2, done2 := start(t, m2)
	defer cancel2()
	collect(t, b2)
	stop(t, cancel2, done2)
	if len(b2.creates) != 0 {
		t.Fatal("сохранённые комнаты заменены новыми")
	}
}
func TestГотовыеСсылкиБезСоздания(t *testing.T) {
	o := options(t.TempDir())
	o.AutoCreate = false
	for i := 1; i <= 5; i++ {
		o.Links = append(o.Links, fmt.Sprintf("https://telemost.yandex.ru/j/20000%d", i))
	}
	b := fake(o.StateFile)
	m, e := New(o, b)
	if e != nil {
		t.Fatal(e)
	}
	cancel, done := start(t, m)
	defer cancel()
	collect(t, b)
	stop(t, cancel, done)
	if len(b.creates) != 0 {
		t.Fatal("при готовых ссылках вызвано создание")
	}
}
func TestПроверкаКонфигурацииИСостояния(t *testing.T) {
	for _, link := range []string{"http://telemost.yandex.ru/j/123", "https://example.com/j/123", "https://telemost.yandex.ru.evil/j/123", "https://telemost.yandex.ru/j/123?x=1", "https://x@yandex.ru/j/123"} {
		if _, e := NormalizeLink(link); e == nil {
			t.Fatal("неверная ссылка принята")
		}
	}
	o := options(t.TempDir())
	o.AutoCreate = false
	if _, e := New(o, nil); e == nil {
		t.Fatal("приняты недостающие ссылки")
	}
	o.AutoCreate = true
	o.Links = []string{"https://telemost.yandex.ru/j/123", "https://telemost.yandex.ru/j/123/"}
	if _, e := New(o, nil); e == nil {
		t.Fatal("повтор ссылки принят")
	}
	o.Links = nil
	os.WriteFile(o.StateFile, []byte("повреждение"), 0600)
	if _, e := New(o, nil); e == nil {
		t.Fatal("повреждённое состояние проигнорировано")
	}
}

func TestПятьдесятКомнатСохраняютсяБезОбрезания(t *testing.T) {
	o := options(t.TempDir())
	o.Sessions = 50
	o.AutoCreate = false
	o.Links = nil
	for i := 0; i < 50; i++ {
		o.Links = append(o.Links, fmt.Sprintf("https://telemost.yandex.ru/j/%d", 500000+i))
	}
	m, e := New(o, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.saveLocked(); e != nil {
		t.Fatal(e)
	}
	m2, e := New(o, nil)
	if e != nil || len(m2.Snapshot()) != 50 {
		t.Fatal("состояние пятидесяти комнат не восстановилось")
	}
	b, e := os.ReadFile(o.LinksFile)
	if e != nil || strings.Count(string(b), "https://") != 50 {
		t.Fatal("экспорт ссылок обрезан")
	}
	o.Sessions = 51
	if _, e = New(o, nil); e == nil {
		t.Fatal("верхняя граница не проверяется")
	}
}
