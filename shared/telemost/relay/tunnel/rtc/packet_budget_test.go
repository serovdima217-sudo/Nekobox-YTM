// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package rtc

import (
	"bytes"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"testing"
	"whitelist-bypass/relay/dup"
	"whitelist-bypass/relay/tunnel"
)

func TestМаксимальныйСегментНеДробитсяНаRTP(t *testing.T) {
	obf, err := tunnel.NewTunnelObfuscator([]byte("тестовый-ключ"))
	if err != nil {
		t.Fatal(err)
	}
	for _, duplicate := range []bool{false, true} {
		mtu := kcpSegmentMTU
		envelope := 0
		if duplicate {
			mtu -= dup.HeaderSize
			envelope = dup.HeaderSize
		}
		frame := obf.EncodeData(bytes.Repeat([]byte{1}, mtu+1+envelope))
		packetizer := rtp.NewPacketizer(1200, 96, 1, &codecs.VP8Payloader{EnablePictureID: true}, rtp.NewRandomSequencer(), 90000)
		for i := 0; i < 200; i++ {
			packets := packetizer.Packetize(frame, 37)
			if len(packets) != 1 {
				t.Fatalf("сегмент разделён на %d RTP-пакетов", len(packets))
			}
			if packets[0].MarshalSize() > 1200 {
				t.Fatal("превышен RTP MTU")
			}
		}
	}
}
