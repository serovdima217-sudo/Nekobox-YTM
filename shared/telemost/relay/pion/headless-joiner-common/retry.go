// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package joiner

import (
	"time"

	"whitelist-bypass/relay/common"
)

const (
	reconnectInitialDelay = time.Second
	reconnectMaxDelay     = 16 * time.Second
)

func waitReconnectBackoff(attempt int, logFn func(string, ...any), logPrefix string, stopCh <-chan struct{}, isClosed func() bool) bool {
	delay := common.BackoffWithJitter(attempt, reconnectInitialDelay, reconnectMaxDelay)
	logFn("%s: waiting %s before reconnect", logPrefix, delay)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return !isClosed()
	case <-stopCh:
		return false
	}
}
