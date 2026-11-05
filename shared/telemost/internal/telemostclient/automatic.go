// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"whitelist-bypass/internal/roomlease"
	"whitelist-bypass/relay/bond"
	"whitelist-bypass/relay/pion"
	joiner "whitelist-bypass/relay/pion/headless-joiner-common"
	"whitelist-bypass/relay/tunnel"
	"whitelist-bypass/relay/tunnel/rtc"
)

// В сборку входят только приглашения. Cookie и доступ к SSH здесь отсутствуют.
//
//go:embed invitations.txt
var automaticInvitations string

const pairedFPS, pairedBatch = 12, 40

func automaticGroups(text string) ([][]string, error) {
	links := strings.Fields(text)
	if len(links) == 0 || len(links)%bond.PairedLanes != 0 {
		return nil, fmt.Errorf("пул приглашений должен состоять из полных четвёрок")
	}
	seen := map[string]bool{}
	groups := [][]string{}
	for i := 0; i < len(links); i += bond.PairedLanes {
		group := links[i : i+bond.PairedLanes]
		if _, e := bond.GroupKey(group); e != nil {
			return nil, e
		}
		for _, link := range group {
			if seen[link] {
				return nil, fmt.Errorf("приглашение повторяется между четвёрками")
			}
			seen[link] = true
		}
		groups = append(groups, group)
	}
	return groups, nil
}

// RunAutomatic — единственная точка входа мобильной сборки: флаги не нужны.
func RunAutomatic() {
	if len(os.Args) != 1 {
		log.Fatal("Мобильный клиент запускается без параметров")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	debug.SetMemoryLimit(128 << 20)
	if e := configureDNS(""); e != nil {
		log.Fatal(e)
	}
	if e := RunAutomaticContext(ctx, "127.0.0.1:9999"); e != nil {
		log.Printf("Клиент остановлен: %v", e)
	}
}

// RunAutomaticContext позволяет испытательному Linux-клиенту иметь отдельный
// локальный порт. В мобильной точке входа адрес жёстко задан: 127.0.0.1:9999.
func RunAutomaticFourContext(ctx context.Context, addr string) error {
	groups, e := automaticGroups(automaticInvitations)
	if e != nil {
		return e
	}
	for ctx.Err() == nil {
		for _, index := range rand.Perm(len(groups)) {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("Проверяю свободную четвёрку %d/%d через Телемост", index+1, len(groups))
			active, e := runPairedAttempt(ctx, groups[index], addr, index+1)
			if e != nil {
				return e
			}
			if active {
				break
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}

func runPairedAttempt(ctx context.Context, links []string, addr string, group int) (bool, error) {
	return runReliableAttempt(ctx, links, addr, group, nil)
}

func runReliableAttempt(ctx context.Context, links []string, addr string, group int, profile *bond.SelectiveProfile, hooks ...func(*bond.Transport)) (active bool, err error) {
	return runReliableAttemptSelected(ctx, links, addr, group, profile, nil, hooks...)
}

func runReliableAttemptSelected(ctx context.Context, links []string, addr string, group int, profile *bond.SelectiveProfile, selectCandidate func() bool, hooks ...func(*bond.Transport)) (active bool, err error) {
	key, e := bond.GroupKey(links)
	if e != nil {
		return false, e
	}
	var t *bond.Transport
	if profile != nil {
		t = bond.NewSelective(key, false, log.Printf)
		t.SetMode(profile.Mode)
	} else {
		t = bond.NewPaired(key, false, log.Printf, 80*time.Millisecond)
	}
	owner := roomlease.Owner(t.ClientIdentifier())
	name := roomlease.NameForWidth(key, owner, len(links))
	rb := tunnel.NewRelayBridge(t, "joiner", rtc.KCPStreamReadSize, log.Printf)
	workCtx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	rejected := make(chan struct{}, 1)
	defer func() {
		releaseCtx, stop := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		t.ReleaseLease(releaseCtx)
		stop()
		cancel()
		t.Close()
		rb.Close()
		workers.Wait()
		stats, _ := json.Marshal(t.Stats())
		log.Printf("Итог каналов: %s", stats)
	}()
	startLane := func(i int) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for workCtx.Err() == nil {
				inner := joiner.NewTelemostHeadlessJoiner(log.Printf, resolveHostname, cliStatusEmitter{}, nil, pion.AddTunnelTracks, pion.ReadTrack)
				if i == 0 {
					inner.OnAdmissionRejected = func() {
						select {
						case rejected <- struct{}{}:
						default:
						}
					}
				}
				var rawMu sync.Mutex
				var current tunnel.DataTunnel
				inner.OnConnected = func(raw tunnel.DataTunnel) {
					rawMu.Lock()
					defer rawMu.Unlock()
					select {
					case <-t.Done():
						return
					default:
					}
					current = raw
					raw.SetOnData(func(data []byte) {
						if !tunnel.LooksLikeRelayFrame(data) {
							t.Handle(i, raw, data)
							return
						}
						tunnel.DecodeFrames(data, func(id uint32, kind byte, p []byte) {
							if id == bond.LaneConnID && kind == tunnel.MsgData {
								t.Handle(i, raw, p)
							}
							if id == tunnel.ControlConnID && kind == tunnel.MsgConfigAck {
								inner.MarkConfigAcked()
							}
						})
					})
					raw.SetOnClose(func() { t.Detach(i, raw) })
					t.Attach(i, raw)
				}
				stop := context.AfterFunc(workCtx, inner.Interrupt)
				fps, batch := pairedFPS, pairedBatch
				if profile != nil {
					fps, batch = profile.Carrier(i)
				}
				params, _ := json.Marshal(map[string]any{"compactBuffers": true, "singleRun": true, "joinLink": links[i], "displayName": name, "vp8Fps": fps, "vp8Batch": batch, "reliable": true, "dualTrack": false, "delayedDuplicate": false})
				inner.RunWithParams(string(params))
				inner.Interrupt()
				stop()
				rawMu.Lock()
				if current != nil {
					t.Detach(i, current)
				}
				rawMu.Unlock()
				inner.Close()
				select {
				case <-workCtx.Done():
					return
				case <-t.Done():
					return
				case <-time.After(2 * time.Second):
				}
				log.Printf("Переподключаю только сессию %d; общий поток сохраняется", i+1)
			}
		}()
	}
	// Только первая комната подаёт заявку. Три остальных не подключаются, пока
	// сервер не занял атомарно всю четвёрку и не подтвердил первый медиапуть.
	startLane(0)
	first := time.NewTimer(60 * time.Second)
	defer first.Stop()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	for !t.Stats()[0].Verified {
		select {
		case <-ctx.Done():
			return false, nil
		case <-t.Done():
			return false, nil
		case <-rejected:
			log.Printf("Набор %d занята; проверю следующую", group)
			return false, nil
		case <-first.C:
			log.Printf("Набор %d не подтвердила бронь; проверю следующую", group)
			return false, nil
		case <-poll.C:
		}
	}
	// Только победитель подбора включает резерв и локальный SOCKS.
	if selectCandidate != nil && !selectCandidate() {
		return false, nil
	}
	for i := 1; i < len(links); i++ {
		startLane(i)
	}
	readyTimeout := time.NewTimer(75 * time.Second)
	defer readyTimeout.Stop()
	select {
	case <-ctx.Done():
		return false, nil
	case <-t.Done():
		return false, nil
	case <-readyTimeout.C:
		log.Printf("Не все сессии набора прошли проверку")
		return false, nil
	case <-t.Ready():
	}
	rb.MarkReady()
	for _, hook := range hooks {
		if hook != nil {
			hook(t)
		}
	}
	listen := make(chan error, 1)
	go func() { listen <- rb.ListenSOCKS(addr) }()
	if profile != nil {
		log.Printf("ОБЪЕДИНЕНИЕ ГОТОВО: основная + облегчённая резервная; набор %d; основной %d/%d, резерв %d/%d; SOCKS5 %s", group, profile.MainFPS, profile.MainBatch, profile.ReserveFPS, profile.ReserveBatch, addr)
	} else {
		log.Printf("ОБЪЕДИНЕНИЕ ГОТОВО: 2 основные + 2 резервные; четвёрка %d; 12 FPS, половинная интенсивность; только камера; SOCKS5 %s", group, addr)
	}
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return true, nil
		case <-t.Done():
			return true, nil
		case e := <-listen:
			return true, e
		case <-tick.C:
			stats, _ := json.Marshal(t.Stats())
			log.Printf("Статистика каналов: %s", stats)
		}
	}
}
