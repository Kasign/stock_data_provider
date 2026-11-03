package cache

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRedisSetRejectsTTLThatRoundsToZeroMilliseconds(t *testing.T) {
	r := NewRedis("127.0.0.1:1", "", "", 0, time.Second)
	if err := r.Set(context.Background(), "k", []byte("v"), 500*time.Microsecond); err == nil {
		t.Fatal("TTL below one millisecond must not silently become Redis PX 0")
	}
}

func TestReadRESPRequiresCRLFForLinesAndBulkPayloads(t *testing.T) {
	for _, fixture := range []string{
		"+PONG\n",
		"$3\r\nabcxx",
	} {
		if _, err := readRESP(bufio.NewReader(strings.NewReader(fixture))); err == nil {
			t.Fatalf("malformed RESP fixture accepted: %q", fixture)
		}
	}
}

func TestWriteAllHandlesShortWrites(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	payload := []byte("abcdef")
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, len(payload))
		_, _ = right.Read(buf)
		got <- string(buf)
	}()
	if err := writeAll(left, payload); err != nil {
		t.Fatal(err)
	}
	if value := <-got; value != string(payload) {
		t.Fatalf("got %q, want %q", value, payload)
	}
}
