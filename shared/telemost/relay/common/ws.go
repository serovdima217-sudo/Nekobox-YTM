// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package common

import (
	"time"

	"github.com/kulikov0/headless-client/websocket"
)

func CloseWS(ws *websocket.Conn) {
	if ws == nil {
		return
	}
	ws.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second))
	ws.Close()
}
