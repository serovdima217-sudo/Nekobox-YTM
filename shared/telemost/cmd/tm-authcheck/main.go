// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

// Проверка сессии без вывода переданного секрета.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	headless "github.com/kulikov0/headless-client"
	"net/http"
	"os"
	"strings"
	"time"
	"whitelist-bypass/relay/common"
	tm "whitelist-bypass/relay/telemost"
)

func main() {
	secret, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fmt.Println("Не получен секрет через стандартный ввод")
		os.Exit(2)
	}
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, "Session_id=") {
		secret = "Session_id=" + secret
	}
	c := tm.Client{Cookie: secret, AppVersion: "211.2.0", HTTP: &http.Client{Timeout: 30 * time.Second, Transport: common.ChromeTransport(headless.TLSOptions{})}}
	b, code, err := c.Do("POST", "/conferences?next_gen_media_platform_allowed=true", struct{}{})
	if err != nil {
		fmt.Println("Сетевая ошибка при проверке авторизации")
		os.Exit(1)
	}
	var v struct {
		URI   string `json:"uri"`
		Error string `json:"error"`
	}
	json.Unmarshal(b, &v)
	fmt.Printf("HTTP: %d; конференция создана: %t\n", code, v.URI != "")
	if v.URI != "" {
		f, e := os.OpenFile("test-room.json", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			os.Exit(1)
		}
		json.NewEncoder(f).Encode(map[string]string{"uri": v.URI})
		f.Close()
	}
	if code != 200 && code != 201 {
		os.Exit(1)
	}
}
