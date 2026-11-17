package baostock

import (
	"context"
	"errors"
	"net"
	"testing"

	"example.com/stock-analysis-flow/internal/marketdata/transport"
)

func TestClientUsesInjectedDialContext(t *testing.T) {
	wantErr := errors.New("blocked by root dial policy")
	var gotNetwork, gotAddress string
	dialContext := transport.DialContextFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("injected dial context must have a deadline")
		}
		gotNetwork, gotAddress = network, address
		return nil, wantErr
	})
	client := NewClient(Config{
		Host: "baostock.example.test", Port: 10030, DialContext: dialContext,
	})
	err := client.ensureLoggedInLocked(context.Background())
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("ensureLoggedInLocked() error = %v, want injected dial error", err)
	}
	if gotNetwork != "tcp" || gotAddress != "baostock.example.test:10030" {
		t.Fatalf("dial target network=%q address=%q", gotNetwork, gotAddress)
	}
}
