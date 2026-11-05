//go:build linux && !android

// SPDX-License-Identifier: GPL-3.0-or-later

package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProcessPathAndPID(t *testing.T) {
	root := t.TempDir()
	fd := filepath.Join(root, "12345", "fd")
	if err := os.MkdirAll(fd, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("socket:[777]", filepath.Join(fd, "7")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/test/launcher (deleted)", filepath.Join(root, "12345", "exe")); err != nil {
		t.Fatal(err)
	}
	pid, path, err := resolveProcessByProcSearchAt(root, 777, uint32(os.Getuid()))
	if err != nil || pid != 12345 || path != "/test/launcher" {
		t.Fatalf("Неверный PID/путь: %d %q %v", pid, path, err)
	}
	if _, _, err := resolveProcessByProcSearchAt(root, 778, uint32(os.Getuid())); err == nil {
		t.Fatal("Посторонний socket inode не должен совпадать")
	}
	if _, _, err := resolveProcessByProcSearchAt(root, 777, uint32(os.Getuid())+1); err == nil {
		t.Fatal("Чужой UID не должен совпадать")
	}
}
