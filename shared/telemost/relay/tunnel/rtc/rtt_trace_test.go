// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package rtc

import (
	"encoding/binary"
	"testing"
	"time"
)

func tracePacket(command byte, sequence uint32) []byte {
	p := make([]byte, 24)
	p[4] = command
	binary.LittleEndian.PutUint32(p[12:16], sequence)
	return p
}

func TestОценкаRTTИсключаетПовторныеПередачи(t *testing.T) {
	s := &trackKCPSession{}
	s.traceOutput(tracePacket(81, 1))
	mark := s.trace[1]
	mark.at = time.Now().Add(-200 * time.Millisecond)
	s.trace[1] = mark
	s.traceInput(tracePacket(82, 1))
	if s.rttMillis.Load() < 180 || s.rttMillis.Load() > 1000 {
		t.Fatal("неверно измерен RTT")
	}
	old := s.rttMillis.Load()
	s.traceOutput(tracePacket(81, 2))
	s.traceOutput(tracePacket(81, 2))
	s.traceInput(tracePacket(82, 2))
	if s.rttMillis.Load() != old {
		t.Fatal("повторная передача исказила RTT")
	}
	if len(s.trace) != 0 {
		t.Fatal("подтверждённые пакеты остались в карте")
	}
}

func TestКартаДиагностикиОграничена(t *testing.T) {
	s := &trackKCPSession{}
	for i := 0; i < 2*kcpWindowCeiling+10; i++ {
		s.traceOutput(tracePacket(81, uint32(i)))
	}
	if len(s.trace) > 2*kcpWindowCeiling {
		t.Fatal("карта диагностики превысила предел")
	}
}
