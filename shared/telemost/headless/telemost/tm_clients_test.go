// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredClientCountUsesExactlyTwoRooms(t *testing.T) {
	var links []string
	for i := 0; i < 50; i++ {
		links = append(links, fmt.Sprintf("https://telemost.yandex.ru/j/%d", 10000000000000+i))
	}
	for _, count := range []int{1, 2, 3, 4, 5, 25} {
		data, _ := json.Marshal(map[string]any{"clients": count, "links": links})
		path := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(path, data, 0600)
		c, err := readManagedConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if c.Sessions != 2*count || len(c.Links) != 2*count || c.BondSize != 2 || !c.SelectiveSessions || c.AutoCreate {
			t.Fatalf("Некорректное число активных комнат: %+v", c)
		}
	}
}
func TestInvalidClientCountCannotCreateNewRooms(t *testing.T) {
	for _, value := range []string{"0", "-1", "26", "null", "true"} {
		data := `{"clients":` + value + `,"links":["https://telemost.yandex.ru/j/12345","https://telemost.yandex.ru/j/67890"]}`
		path := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(path, []byte(data), 0600)
		if _, err := readManagedConfig(path); err == nil {
			t.Fatalf("Принято неверное число клиентов: %s", value)
		}
	}
}
