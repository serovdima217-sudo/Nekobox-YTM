// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

// Package roommgr управляет независимыми комнатами в одном процессе.
package roommgr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const MaxRooms = 50

type Room struct {
	Slot int    `json:"slot"`
	Link string `json:"link"`
}
type Backend interface {
	Create(context.Context, int) (string, error)
	Run(context.Context, int, string) error
}
type Options struct {
	Sessions                int
	AutoCreate              bool
	Links                   []string
	StateFile, LinksFile    string
	RetryDelay, CreateDelay time.Duration
	OnStatus                func(int, string)
}
type Manager struct {
	o          Options
	b          Backend
	mu         sync.Mutex
	rooms      map[int]string
	createGate chan struct{}
}

func NormalizeLink(s string) (string, error) {
	u, e := url.Parse(strings.TrimSpace(s))
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return "", fmt.Errorf("нужна HTTPS-ссылка на Телемост")
	}
	switch strings.ToLower(u.Hostname()) {
	case "telemost.yandex.ru", "telemost.yandex.com", "telemost.yandex.net":
	default:
		return "", fmt.Errorf("ссылка ведёт не на Телемост")
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	if !strings.HasPrefix(u.Path, "/j/") || len(strings.TrimPrefix(u.Path, "/j/")) < 3 || strings.Contains(strings.TrimPrefix(u.Path, "/j/"), "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("ожидается ссылка вида https://telemost.yandex.ru/j/идентификатор")
	}
	return u.String(), nil
}
func New(o Options, b Backend) (*Manager, error) {
	if o.Sessions < 1 || o.Sessions > MaxRooms || len(o.Links) > o.Sessions {
		return nil, fmt.Errorf("число комнат должно быть от 1 до 50, ссылок не больше числа комнат")
	}
	if o.StateFile == "" || o.LinksFile == "" || o.StateFile == o.LinksFile {
		return nil, fmt.Errorf("нужны разные файлы состояния и ссылок")
	}
	if o.RetryDelay <= 0 {
		o.RetryDelay = 10 * time.Second
	}
	if o.CreateDelay <= 0 {
		o.CreateDelay = 3 * time.Second
	}
	m := &Manager{o: o, b: b, rooms: make(map[int]string), createGate: make(chan struct{}, 1)}
	data, err := os.ReadFile(o.StateFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		var rooms []Room
		if json.Unmarshal(data, &rooms) != nil {
			return nil, fmt.Errorf("файл комнат повреждён; автоматическое создание остановлено")
		}
		for _, r := range rooms {
			link, e := NormalizeLink(r.Link)
			if e != nil || r.Slot < 1 || r.Slot > MaxRooms || m.rooms[r.Slot] != "" {
				return nil, fmt.Errorf("неверная или повторяющаяся запись комнаты")
			}
			m.rooms[r.Slot] = link
		}
	}
	for i, s := range o.Links {
		if strings.TrimSpace(s) == "" {
			continue
		}
		link, e := NormalizeLink(s)
		if e != nil {
			return nil, e
		}
		if old := m.rooms[i+1]; old != "" && old != link {
			return nil, fmt.Errorf("ссылка места %d отличается от сохранённой; измените состояние явно", i+1)
		}
		m.rooms[i+1] = link
	}
	seen := map[string]bool{}
	for _, s := range m.rooms {
		if seen[s] {
			return nil, fmt.Errorf("одна ссылка назначена разным местам")
		}
		seen[s] = true
	}
	if !o.AutoCreate {
		for i := 1; i <= o.Sessions; i++ {
			if m.rooms[i] == "" {
				return nil, fmt.Errorf("для места %d нет ссылки, а автоматическое создание отключено", i)
			}
		}
	}
	return m, nil
}
func atomicWrite(path string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".комнаты-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}

// Состояние является главным файлом. Список ссылок восстанавливается из него при запуске.
func (m *Manager) saveLocked() error {
	var rooms []Room
	var text strings.Builder
	for i := 1; i <= MaxRooms; i++ {
		if link := m.rooms[i]; link != "" {
			rooms = append(rooms, Room{i, link})
			if i <= m.o.Sessions {
				fmt.Fprintln(&text, link)
			}
		}
	}
	b, e := json.MarshalIndent(rooms, "", "  ")
	if e != nil {
		return e
	}
	if e = atomicWrite(m.o.StateFile, b); e != nil {
		return e
	}
	return atomicWrite(m.o.LinksFile, []byte(text.String()))
}
func (m *Manager) Snapshot() []Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Room
	for i := 1; i <= m.o.Sessions; i++ {
		if s := m.rooms[i]; s != "" {
			out = append(out, Room{i, s})
		}
	}
	return out
}
func wait(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
func (m *Manager) status(slot int, s string) {
	if m.o.OnStatus != nil {
		m.o.OnStatus(slot, s)
	}
}
func (m *Manager) Run(parent context.Context) error {
	if m.b == nil {
		return fmt.Errorf("не задан обработчик комнат")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	m.mu.Lock()
	err := m.saveLocked()
	m.mu.Unlock()
	if err != nil {
		return err
	}
	fatal := make(chan error, 1)
	var wg sync.WaitGroup
	for slot := 1; slot <= m.o.Sessions; slot++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			delay := m.o.RetryDelay
			for ctx.Err() == nil {
				m.mu.Lock()
				link := m.rooms[slot]
				m.mu.Unlock()
				if link == "" {
					select {
					case m.createGate <- struct{}{}:
					case <-ctx.Done():
						return
					}
					m.status(slot, "создание комнаты")
					created, e := m.b.Create(ctx, slot)
					if e == nil {
						created, e = NormalizeLink(created)
						if e != nil {
							select {
							case fatal <- fmt.Errorf("сервис вернул некорректную ссылку; создание остановлено"):
							default:
							}
							cancel()
						}
						if e == nil {
							m.mu.Lock()
							for _, old := range m.rooms {
								if old == created {
									e = fmt.Errorf("сервис вернул уже существующую комнату")
									break
								}
							}
							if e == nil {
								m.rooms[slot] = created
								if err := m.saveLocked(); err != nil {
									e = err
								}
							}
							m.mu.Unlock()
							if e != nil {
								select {
								case fatal <- fmt.Errorf("не удалось безопасно сохранить новую комнату: %w", e):
								default:
								}
								cancel()
							}
						}
					}
					if e != nil {
						m.status(slot, "не удалось создать комнату; проверьте cookie и ограничения сервиса")
					}
					// Интервал сериализован вместе с созданием, включая ошибки API.
					wait(ctx, m.o.CreateDelay)
					<-m.createGate
					if e != nil {
						pause := delay
						if pause < time.Minute {
							pause = time.Minute
						}
						if !wait(ctx, pause) {
							return
						}
						continue
					}
					link = created
				}
				if ctx.Err() != nil {
					return
				}
				m.status(slot, "подключение к сохранённой комнате")
				started := time.Now()
				m.b.Run(ctx, slot, link)
				if ctx.Err() != nil {
					return
				}
				m.status(slot, "сессия завершена; переподключается только эта комната")
				if time.Since(started) > time.Minute {
					delay = m.o.RetryDelay
				}
				if !wait(ctx, delay) {
					return
				}
				if delay < time.Minute {
					delay *= 2
					if delay > time.Minute {
						delay = time.Minute
					}
				}
			}
		}(slot)
	}
	wg.Wait()
	select {
	case e := <-fatal:
		return e
	default:
		return nil
	}
}
