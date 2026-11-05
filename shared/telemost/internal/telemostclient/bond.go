// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sync"
	"time"

	"whitelist-bypass/relay/bond"
	"whitelist-bypass/relay/pion"
	joiner "whitelist-bypass/relay/pion/headless-joiner-common"
	"whitelist-bypass/relay/tunnel"
	"whitelist-bypass/relay/tunnel/rtc"
)

func readBondLinks(path string) ([]string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, fmt.Errorf("не удалось прочитать файл ссылок")
	}
	links := regexp.MustCompile(`https://telemost\.yandex\.(?:ru|com|net)/j/[A-Za-z0-9_-]+`).FindAllString(string(b), -1)
	if _, e = bond.GroupKey(links); e != nil {
		return nil, e
	}
	return links, nil
}

func runBond(ctx context.Context, links []string, addr, name string, fps, batch int, duplicate bool, mode bond.Mode) {
	for ctx.Err() == nil {
		runBondOnce(ctx, links, addr, name, fps, batch, duplicate, mode)
		if ctx.Err() != nil {
			return
		}
		log.Printf("Переподключаю всю группу через пять секунд; прерванные TCP нужно открыть заново")
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func runBondOnce(ctx context.Context, links []string, addr, name string, fps, batch int, duplicate bool, mode bond.Mode) {
	key, e := bond.GroupKey(links)
	if e != nil {
		log.Printf("Ошибка группы: %v", e)
		return
	}
	t := bond.New(key, false, log.Printf, len(links))
	t.SetMode(mode)
	log.Printf("Режим объединения: %s; распределение по фактической доставке, задержке и джиттеру", mode)
	rb := tunnel.NewRelayBridge(t, "joiner", rtc.KCPStreamReadSize, log.Printf)
	t.SetOnClose(rb.Close)
	var inners [bond.Lanes]*joiner.TelemostHeadlessJoiner
	var laneMu [bond.Lanes]sync.Mutex
	var active [bond.Lanes]tunnel.DataTunnel
	var workers sync.WaitGroup
	for i, link := range links {
		i, link := i, link
		inner := joiner.NewTelemostHeadlessJoiner(log.Printf, resolveHostname, cliStatusEmitter{}, nil, pion.AddTunnelTracks, pion.ReadTrack)
		inners[i] = inner
		inner.OnConnected = func(raw tunnel.DataTunnel) {
			laneMu[i].Lock()
			defer laneMu[i].Unlock()
			select {
			case <-t.Done():
				return
			default:
			}
			if active[i] != nil {
				t.Close()
				return
			}
			active[i] = raw
			raw.SetOnData(func(data []byte) {
				if tunnel.LooksLikeRelayFrame(data) {
					tunnel.DecodeFrames(data, func(id uint32, kind byte, payload []byte) {
						if id == bond.LaneConnID && kind == tunnel.MsgData {
							t.Handle(i, raw, payload)
							return
						}
						if id == tunnel.ControlConnID && kind == tunnel.MsgConfigAck {
							inner.MarkConfigAcked()
						}
					})
					return
				}
				t.Handle(i, raw, data)
			})
			raw.SetOnClose(func() { t.Detach(i, raw) })
			t.Attach(i, raw)
		}
		params, _ := json.Marshal(map[string]any{"compactBuffers": true, "joinLink": link, "displayName": fmt.Sprintf("%s-%d", name, i+1), "vp8Fps": fps, "vp8Batch": batch, "reliable": true, "dualTrack": duplicate, "delayedDuplicate": duplicate})
		workers.Add(1)
		go func() {
			defer workers.Done()
			select {
			case <-ctx.Done():
				return
			case <-t.Done():
				return
			case <-time.After(time.Duration(i) * 250 * time.Millisecond):
			}
			inner.RunWithParams(string(params))
			select {
			case <-t.Done():
			default:
				t.Close()
			}
		}()
	}
	defer func() {
		rb.Close()
		t.Close()
		for i, inner := range inners[:len(links)] {
			laneMu[i].Lock()
			laneMu[i].Unlock()
			inner.Close()
		}
		workers.Wait()
		stats, _ := json.Marshal(t.Stats())
		log.Printf("Итог каналов: %s", stats)
	}()
	select {
	case <-ctx.Done():
		return
	case <-t.Done():
		return
	case <-t.Ready():
	}
	rb.MarkReady()
	listenDone := make(chan error, 1)
	go func() { listenDone <- rb.ListenSOCKS(addr) }()
	log.Printf("ОБЪЕДИНЕНИЕ ГОТОВО: %d/%d, SOCKS5 %s; один поток распределяется по каналам", len(links), len(links), addr)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.Done():
			return
		case e := <-listenDone:
			if e != nil {
				log.Printf("Ошибка локального SOCKS: %v", e)
			}
			return
		case <-ticker.C:
			stats, _ := json.Marshal(t.Stats())
			log.Printf("Статистика каналов: %s", stats)
		}
	}
}
