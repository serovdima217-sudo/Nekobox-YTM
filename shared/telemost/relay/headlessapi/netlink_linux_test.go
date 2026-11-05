// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

//go:build linux && amd64

package headlessapi

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/kulikov0/headless-client/webrtc"
	"golang.org/x/sys/unix"
	"whitelist-bypass/relay/common"
)

// Проверяем именно запрет AF_NETLINK, а не просто успешную сборку Linux.
func TestWebRTCWithoutNetlink(t *testing.T) {
	if os.Getenv("TM_TEST_DENY_NETLINK") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestWebRTCWithoutNetlink$", "-test.v")
		cmd.Env = append(os.Environ(), "TM_TEST_DENY_NETLINK=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Проверка в отдельном процессе: %v\n%s", err, out)
		}
		t.Log(string(out))
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// socket(AF_NETLINK,...) -> EPERM; остальные системные вызовы разрешены.
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jf: 3, K: uint32(unix.SYS_SOCKET)},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 16},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jf: 1, K: uint32(unix.AF_NETLINK)},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	prog := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	_, _, e := unix.RawSyscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&prog)))
	if e != 0 {
		t.Fatal(e)
	}
	if _, err := net.Interfaces(); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("Запрет netlink не воспроизведён: %v", err)
	}
	api, err := WebRTCAPI(Options{Configure: func(e *webrtc.SettingEngine) { e.SetNet(&common.AndroidNet{}) }})
	if err != nil {
		t.Fatal(err)
	}
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	channel, err := pc.CreateDataChannel("проверка", nil)
	if err != nil {
		t.Fatal(err)
	}
	channel.OnOpen(func() {
		if err := channel.SendText("проверка без netlink"); err != nil {
			t.Error(err)
		}
	})
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("ICE при запрете netlink: %v", err)
	}
	done := webrtc.GatheringCompletePromise(pc)
	if err = pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("ICE не завершил поиск адресов")
	}
	if !strings.Contains(pc.LocalDescription().SDP, "a=candidate:") {
		t.Fatal("Не собран ни один ICE-кандидат")
	}
	answerPC, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer answerPC.Close()
	delivered := make(chan string, 1)
	answerPC.OnDataChannel(func(channel *webrtc.DataChannel) {
		channel.OnMessage(func(message webrtc.DataChannelMessage) { delivered <- string(message.Data) })
	})
	if err = answerPC.SetRemoteDescription(*pc.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	answer, err := answerPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("Создание ответа при запрете netlink: %v", err)
	}
	answerDone := webrtc.GatheringCompletePromise(answerPC)
	if err = answerPC.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-answerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Не собраны кандидаты ответа")
	}
	if err = pc.SetRemoteDescription(*answerPC.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-delivered:
		if value != "проверка без netlink" {
			t.Fatal("Искажён SCTP-пакет")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Не установлено UDP/DTLS/SCTP-соединение")
	}
	t.Log("AF_NETLINK запрещён: CreateOffer, CreateAnswer, ICE, DTLS и передача SCTP выполнены.")
}
