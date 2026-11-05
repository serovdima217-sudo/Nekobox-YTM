//go:build linux && !android

// SPDX-License-Identifier: GPL-3.0-or-later

package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type ancestorEntry struct {
	parent     uint32
	uid        uint32
	executable string
}

// MatchAncestorProcessPath проверяет только живую цепочку предков того же UID.
// Обычное process_path без специального флага по-прежнему сопоставляется точно.
func MatchAncestorProcessPath(pid uint32, paths map[string]bool) bool {
	return matchAncestorProcessPath(pid, paths, readAncestorEntry)
}

func readAncestorEntry(pid uint32) (ancestorEntry, bool) {
	root := filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10))
	info, err := os.Stat(root)
	if err != nil {
		return ancestorEntry{}, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ancestorEntry{}, false
	}
	raw, err := os.ReadFile(filepath.Join(root, "stat"))
	if err != nil {
		return ancestorEntry{}, false
	}
	end := strings.LastIndexByte(string(raw), ')')
	if end < 0 {
		return ancestorEntry{}, false
	}
	fields := strings.Fields(string(raw[end+1:]))
	if len(fields) < 2 {
		return ancestorEntry{}, false
	}
	parent, err := strconv.ParseUint(fields[1], 10, 32)
	if err != nil {
		return ancestorEntry{}, false
	}
	executable, err := os.Readlink(filepath.Join(root, "exe"))
	if err != nil {
		return ancestorEntry{}, false
	}
	return ancestorEntry{uint32(parent), stat.Uid, normalizeLinuxExecutable(executable)}, true
}

func matchAncestorProcessPath(pid uint32, paths map[string]bool, load func(uint32) (ancestorEntry, bool)) bool {
	if pid <= 1 || len(paths) == 0 {
		return false
	}
	current, ok := load(pid)
	if !ok {
		return false
	}
	owner := current.uid
	seen := map[uint32]bool{pid: true}
	parent := current.parent
	// Ограничение защищает от циклов и чрезмерно длинной цепочки.
	for depth := 0; depth < 64 && parent > 1; depth++ {
		if seen[parent] {
			return false
		}
		seen[parent] = true
		item, ok := load(parent)
		if !ok || item.uid != owner {
			return false
		}
		if paths[normalizeLinuxExecutable(item.executable)] {
			return true
		}
		parent = item.parent
	}
	return false
}
