package provider

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/transport"
)

func TestPytdxUsesInjectedDialContext(t *testing.T) {
	wantErr := errors.New("blocked by root dial policy")
	var gotNetwork, gotAddress string
	dialContext := transport.DialContextFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("injected dial context must have a deadline")
		}
		gotNetwork, gotAddress = network, address
		return nil, wantErr
	})
	p := NewPytdxWithDialContext(time.Second, dialContext, "tdx.example.test")
	_, err := p.dial(context.Background())
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("dial() error = %v, want injected dial error", err)
	}
	if gotNetwork != "tcp" || gotAddress != "tdx.example.test:7709" {
		t.Fatalf("dial target network=%q address=%q", gotNetwork, gotAddress)
	}
}

func TestPytdxLegacyConstructorKeepsDefaultDialBehavior(t *testing.T) {
	p := NewPytdx(time.Second, "127.0.0.1:7709")
	if p.dialContext != nil || len(p.hosts) != 1 || p.hosts[0] != "127.0.0.1:7709" {
		t.Fatalf("legacy constructor changed: %+v", p)
	}
}
