package tunnel

import (
	"context"
	"net"
	"testing"
)

func TestMinecraftSRVResolution(t *testing.T) {
	orig := srvLookupFunc
	defer func() { srvLookupFunc = orig }()
	srvLookupFunc = func(ctx context.Context, host string) ([]*net.SRV, error) {
		switch host {
		case "play.example.net":
			return []*net.SRV{{Target: "node1.example.net.", Port: 26959}}, nil
		case "local-target.example.net":
			return []*net.SRV{{Target: "127.0.0.1.", Port: 9019}}, nil
		default:
			return nil, &net.DNSError{IsNotFound: true}
		}
	}
	if got := resolveMinecraftSRV("play.example.net:25565", nil); got != "node1.example.net:26959" {
		t.Fatalf("ожидалось перенаправление SRV, получено %s", got)
	}
	if got := resolveMinecraftSRV("play.example.net:443", nil); got != "play.example.net:443" {
		t.Fatalf("порт 443 не должен переписываться: %s", got)
	}
	if got := resolveMinecraftSRV("local-target.example.net:25565", nil); got != "local-target.example.net:25565" {
		t.Fatalf("локальный адрес в SRV должен блокироваться: %s", got)
	}
}
