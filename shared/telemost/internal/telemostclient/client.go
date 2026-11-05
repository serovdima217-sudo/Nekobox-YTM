// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"

	"whitelist-bypass/relay/bond"
	"whitelist-bypass/relay/common"
	"whitelist-bypass/relay/pion"
	joiner "whitelist-bypass/relay/pion/headless-joiner-common"
	"whitelist-bypass/relay/tunnel"
	"whitelist-bypass/relay/tunnel/rtc"
)

type cliStatusEmitter struct{}

func (cliStatusEmitter) EmitStatus(status string)   { log.Printf("[status] %s", status) }
func (cliStatusEmitter) EmitStatusError(msg string) { log.Printf("[status] ERROR: %s", msg) }

func resolveHostname(hostname string) (string, error) { return desktopLookup(hostname) }

func Run(mobile bool) {
	common.MaybePrintVersion()
	duplicateDelay := flag.Bool("delayed-duplicate", true, "принимать отложенные копии через демонстрацию экрана")
	bondMode := flag.String("bond-mode", "баланс", "слияние: скорость, баланс, устойчивость")
	linksFile := flag.String("links-file", "", "файл от одной до десяти ссылок для объединения")
	tmLink := flag.String("tm-link", "", "ссылка вашей комнаты Телемоста (обязательно)")
	displayName := flag.String("name", "Joiner", "имя участника")
	socksHost := flag.String("socks-host", common.SocksLocalhostIP, "адрес локального SOCKS5")
	port := 1080
	if mobile {
		port = 9999
	}
	socksPort := flag.Int("socks-port", port, "порт локального SOCKS5")
	socksUser := flag.String("socks-user", "", "имя SOCKS5 (не используется в мобильной сборке)")
	socksPass := flag.String("socks-pass", "", "пароль SOCKS5 (не используется в мобильной сборке)")
	resourceMode := "default"
	if mobile {
		resourceMode = "moderate"
	}
	resources := flag.String("resources", resourceMode, "память: moderate, default, unlimited")
	vp8FPS := flag.Int("vp8-fps", 60, "частота VP8")
	vp8Batch := flag.Int("vp8-batch", 120, "множитель частоты пакетов")
	reliable := flag.Bool("reliable", false, "надёжная передача KCP")
	dualTrack := flag.Bool("dual-track", false, "запросить демонстрационный путь")
	debugFlag := flag.Bool("debug", false, "подробный диагностический журнал")
	dns := flag.String("dns", "", "DNS служебных узлов, например 77.88.8.8:53; домены SOCKS разрешаются на сервере")
	flag.Parse()
	if mobile && (*socksHost != "127.0.0.1" || *socksPort != 9999 || *socksUser != "" || *socksPass != "") {
		log.Fatal("Этот клиент работает только на 127.0.0.1:9999 без авторизации")
	}
	if *vp8FPS < 1 || *vp8FPS > 60 || *vp8Batch < 1 || *vp8Batch > 120 {
		log.Fatal("Допустимы fps 1–60 и batch 1–120")
	}
	if err := configureDNS(*dns); err != nil {
		log.Fatal(err)
	}
	common.Debug = *debugFlag

	if *tmLink == "" && *linksFile == "" {
		log.Fatal("укажите -tm-link со ссылкой комнаты")
	}

	if *linksFile != "" {
		specified := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "resources" {
				specified = true
			}
		})
		if !specified {
			*resources = "default"
		}
	}

	var memLimit int64
	switch *resources {
	case "moderate":
		memLimit = 64 << 20
	case "default":
		memLimit = 128 << 20
	case "unlimited":
		memLimit = 256 << 20
	default:
		log.Fatalf("[config] unknown resources mode: %s", *resources)
	}
	if memLimit > 0 {
		debug.SetMemoryLimit(memLimit)
	}

	if *linksFile != "" {
		if *tmLink != "" {
			log.Fatal("Нельзя одновременно задавать -tm-link и -links-file")
		}
		mode, modeErr := bond.ParseMode(*bondMode)
		if modeErr != nil {
			log.Fatal(modeErr)
		}
		links, e := readBondLinks(*linksFile)
		if e != nil {
			log.Fatal(e)
		}
		explicitFPS, explicitBatch := false, false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "vp8-fps" {
				explicitFPS = true
			}
			if f.Name == "vp8-batch" {
				explicitBatch = true
			}
		})
		if !explicitFPS {
			*vp8FPS = 24
		}
		if !explicitBatch {
			*vp8Batch = 40
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		runBond(ctx, links, fmt.Sprintf("%s:%d", *socksHost, *socksPort), *displayName, *vp8FPS, *vp8Batch, *duplicateDelay, mode)
		return
	}

	inner := joiner.NewTelemostHeadlessJoiner(
		log.Printf,
		resolveHostname,
		cliStatusEmitter{},
		nil,
		pion.AddTunnelTracks,
		pion.ReadTrack,
	)
	var bridgeMu sync.Mutex
	var activeBridge *tunnel.RelayBridge
	stopping := false
	inner.OnConnected = func(tun tunnel.DataTunnel) {
		bridgeMu.Lock()
		defer bridgeMu.Unlock()
		if stopping {
			return
		}
		if activeBridge != nil {
			activeBridge.SwapTunnel(tun)
			return
		}
		readBuf := common.VP8BufSize
		if _, ok := tun.(*rtc.DCTunnel); ok {
			readBuf = common.DCBufSize
		}
		if _, ok := tun.(*rtc.MultiTrackKCPTunnel); ok {
			readBuf = rtc.KCPStreamReadSize
		}
		bridge := tunnel.NewRelayBridgeWithAuth(tun, "joiner", readBuf, log.Printf, *socksUser, *socksPass)
		activeBridge = bridge
		bridge.SetPersistentListener(true)
		bridge.SetOnConfigAck(inner.MarkConfigAcked)
		bridge.MarkReady()
		addr := fmt.Sprintf("%s:%d", *socksHost, *socksPort)
		go func() {
			if err := bridge.ListenSOCKS(addr); err != nil {
				log.Printf("ошибка локального SOCKS: %v", err)
			}
		}()
		fmt.Printf("\n  ТУННЕЛЬ ПОДКЛЮЧЁН\n  socks5 -> %s\n\n", addr)
	}

	params, _ := json.Marshal(struct {
		JoinLink    string `json:"joinLink"`
		DisplayName string `json:"displayName"`
		VP8FPS      int    `json:"vp8Fps"`
		VP8Batch    int    `json:"vp8Batch"`
		Reliable    bool   `json:"reliable"`
		DualTrack   bool   `json:"dualTrack"`
		Duplicate   bool   `json:"delayedDuplicate"`
	}{
		JoinLink:    strings.TrimSpace(*tmLink),
		DisplayName: *displayName,
		VP8FPS:      *vp8FPS,
		VP8Batch:    *vp8Batch,
		Reliable:    *reliable,
		DualTrack:   *dualTrack,
		Duplicate:   *duplicateDelay,
	})

	go inner.RunWithParams(string(params))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Printf("Клиент останавливается")
	bridgeMu.Lock()
	stopping = true
	if activeBridge != nil {
		activeBridge.Close()
	}
	bridgeMu.Unlock()
	inner.Close()
}
