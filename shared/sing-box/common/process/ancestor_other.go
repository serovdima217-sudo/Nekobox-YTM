//go:build !linux || android

// SPDX-License-Identifier: GPL-3.0-or-later

package process

// Наследование процессов поддерживается только нативным Linux-клиентом.
func MatchAncestorProcessPath(pid uint32, paths map[string]bool) bool { return false }
