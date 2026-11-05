//go:build linux && !android

// SPDX-License-Identifier: GPL-3.0-or-later

package process

import "testing"

func TestProcessPathAncestor(t *testing.T) {
	cases := []struct {
		name     string
		tree     map[uint32]ancestorEntry
		selected map[string]bool
		want     bool
	}{
		{"дочерний JRE", map[uint32]ancestorEntry{30: {20, 1000, "/game/jre/bin/java"}, 20: {1, 1000, "/launcher/jre/bin/java"}}, map[string]bool{"/launcher/jre/bin/java": true}, true},
		{"промежуточная оболочка", map[uint32]ancestorEntry{30: {20, 1000, "/game/java"}, 20: {10, 1000, "/usr/bin/sh"}, 10: {1, 1000, "/launcher/java"}}, map[string]bool{"/launcher/java": true}, true},
		{"одноимённый посторонний JRE", map[uint32]ancestorEntry{30: {20, 1000, "/game/java"}, 20: {1, 1000, "/other/java"}, 40: {1, 1000, "/launcher/java"}}, map[string]bool{"/launcher/java": true}, false},
		{"граница UID", map[uint32]ancestorEntry{30: {20, 1000, "/game/java"}, 20: {1, 0, "/launcher/java"}}, map[string]bool{"/launcher/java": true}, false},
		{"заменённый файл предка", map[uint32]ancestorEntry{30: {20, 1000, "/game/java"}, 20: {1, 1000, "/launcher/java (deleted)"}}, map[string]bool{"/launcher/java": true}, true},
		{"исчезнувший предок", map[uint32]ancestorEntry{30: {20, 1000, "/game/java"}}, map[string]bool{"/launcher/java": true}, false},
		{"цикл", map[uint32]ancestorEntry{30: {20, 1000, "/game/java"}, 20: {30, 1000, "/other/java"}}, map[string]bool{"/launcher/java": true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchAncestorProcessPath(30, tc.selected, func(pid uint32) (ancestorEntry, bool) { v, ok := tc.tree[pid]; return v, ok })
			if got != tc.want {
				t.Fatalf("Наследование: получено %v, требуется %v", got, tc.want)
			}
		})
	}
}

func TestProcessPathAncestorBounded(t *testing.T) {
	calls := 0
	got := matchAncestorProcessPath(2, map[string]bool{"/selected": true}, func(pid uint32) (ancestorEntry, bool) { calls++; return ancestorEntry{pid + 1, 1000, "/other"}, true })
	if got || calls > 65 {
		t.Fatalf("Цепочка должна быть ограничена: результат %v, чтений %d", got, calls)
	}
}
