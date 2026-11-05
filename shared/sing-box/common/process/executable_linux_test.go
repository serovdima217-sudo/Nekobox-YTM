//go:build linux

package process

import "testing"

func TestNormalizeLinuxExecutable(t *testing.T) {
	for input, want := range map[string]string{
		"/usr/bin/firefox":                 "/usr/bin/firefox",
		"/usr/bin/firefox (deleted)":       "/usr/bin/firefox",
		"/путь с пробелами/java (deleted)": "/путь с пробелами/java",
		"/usr/bin/not-deleted":             "/usr/bin/not-deleted",
	} {
		if got := normalizeLinuxExecutable(input); got != want {
			t.Errorf("Путь %q: получено %q, ожидалось %q", input, got, want)
		}
	}
}
