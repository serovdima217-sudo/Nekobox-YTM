//go:build linux && !android

// SPDX-License-Identifier: GPL-3.0-or-later

package rule

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/process"
	"github.com/sagernet/sing-box/option"
)

func TestProcessPathChildrenCompatibility(t *testing.T) {
	item := NewProcessPathItem([]string{"/selected"})
	if !item.Match(&adapter.InboundContext{ProcessInfo: &process.Info{ProcessPath: "/selected"}}) {
		t.Fatal("Изменено точное process_path")
	}
	if item.Match(&adapter.InboundContext{ProcessInfo: &process.Info{ProcessPath: "/other"}}) {
		t.Fatal("Посторонний процесс совпал")
	}
	var options option.Rule
	if err := json.Unmarshal([]byte(`{"process_path":["/selected"],"process_path_include_children":true,"action":"route","outbound":"direct"}`), &options); err != nil {
		t.Fatal(err)
	}
	if !options.DefaultOptions.ProcessPathIncludeChildren {
		t.Fatal("Потерян флаг наследования в JSON")
	}
	invalid := option.DefaultRule{}
	invalid.ProcessPathIncludeChildren = true
	if _, err := NewDefaultRule(context.Background(), nil, invalid); err == nil {
		t.Fatal("Наследование без выбранного process_path не должно превращаться в правило для всех")
	}
}

func TestProcessPathChildrenLive(t *testing.T) {
	ppid := os.Getppid()
	if ppid <= 1 {
		t.Skip("Нет живого родителя для проверки")
	}
	parent, err := os.Readlink("/proc/" + strconv.Itoa(ppid) + "/exe")
	if err != nil {
		t.Fatal(err)
	}
	metadata := &adapter.InboundContext{ProcessInfo: &process.Info{ProcessID: uint32(os.Getpid()), ProcessPath: "/other-child-executable"}}
	if NewProcessPathItem([]string{parent}).Match(metadata) {
		t.Fatal("Обычное правило неожиданно распространилось на ребёнка")
	}
	if !NewProcessPathItem([]string{parent}, true).Match(metadata) {
		t.Fatal("Живой дочерний процесс не унаследовал исключение")
	}
	if NewProcessPathItem([]string{"/не-существующий-лаунчер"}, true).Match(metadata) {
		t.Fatal("Посторонняя цепочка ошибочно исключена")
	}
}
