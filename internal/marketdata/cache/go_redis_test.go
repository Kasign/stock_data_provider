package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type fakeGoRedisCommands struct {
	getCommand  *redis.StringCmd
	getKey      string
	setCommand  *redis.StatusCmd
	pingCommand *redis.StatusCmd
	setKey      string
	setValue    interface{}
	setTTL      time.Duration
}

func (f *fakeGoRedisCommands) Get(ctx context.Context, key string) *redis.StringCmd {
	f.getKey = key
	if f.getCommand == nil {
		f.getCommand = redis.NewStringCmd(ctx)
	}
	return f.getCommand
}

func (f *fakeGoRedisCommands) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.StatusCmd {
	f.setKey, f.setValue, f.setTTL = key, value, ttl
	if f.setCommand == nil {
		f.setCommand = redis.NewStatusCmd(ctx)
		f.setCommand.SetVal("OK")
	}
	return f.setCommand
}

func (f *fakeGoRedisCommands) Ping(ctx context.Context) *redis.StatusCmd {
	if f.pingCommand == nil {
		f.pingCommand = redis.NewStatusCmd(ctx)
		f.pingCommand.SetVal("PONG")
	}
	return f.pingCommand
}

func TestGoRedisGetHitAndMiss(t *testing.T) {
	ctx := context.Background()
	client := &fakeGoRedisCommands{getCommand: redis.NewStringCmd(ctx)}
	client.getCommand.SetVal("cached")
	adapter, err := NewGoRedis(client)
	if err != nil {
		t.Fatalf("NewGoRedis() error = %v", err)
	}

	got, found, err := adapter.Get(ctx, "quote:600000")
	if err != nil || !found || string(got) != "cached" || client.getKey != "quote:600000" {
		t.Fatalf("Get() = %q, %v, %v; key = %q", got, found, err, client.getKey)
	}

	miss := &fakeGoRedisCommands{getCommand: redis.NewStringCmd(ctx)}
	miss.getCommand.SetErr(redis.Nil)
	adapter, err = NewGoRedis(miss)
	if err != nil {
		t.Fatalf("NewGoRedis() miss adapter error = %v", err)
	}
	got, found, err = adapter.Get(ctx, "quote:missing")
	if err != nil || found || got != nil || miss.getKey != "quote:missing" {
		t.Fatalf("missing Get() = %q, %v, %v; key = %q; want nil, false, nil", got, found, err, miss.getKey)
	}
}

func TestGoRedisSetPreservesValueAndTTL(t *testing.T) {
	ctx := context.Background()
	client := &fakeGoRedisCommands{}
	adapter, err := NewGoRedis(client)
	if err != nil {
		t.Fatalf("NewGoRedis() error = %v", err)
	}
	value := []byte("payload")
	ttl := 1500 * time.Millisecond
	if err := adapter.Set(ctx, "bars:600000", value, ttl); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	gotValue, ok := client.setValue.([]byte)
	if client.setKey != "bars:600000" || !ok || string(gotValue) != "payload" || client.setTTL != ttl {
		t.Fatalf("Set() arguments = key:%q value:%v ttl:%s", client.setKey, client.setValue, client.setTTL)
	}
	if err := adapter.Set(ctx, "bars:600000", value, time.Microsecond); err == nil {
		t.Fatal("Set() must reject TTL below 1ms")
	}
}

func TestGoRedisGetAndSetPropagateErrors(t *testing.T) {
	ctx := context.Background()
	getErr := errors.New("redis GET failed")
	getClient := &fakeGoRedisCommands{getCommand: redis.NewStringCmd(ctx)}
	getClient.getCommand.SetErr(getErr)
	getAdapter, err := NewGoRedis(getClient)
	if err != nil {
		t.Fatalf("NewGoRedis() GET adapter error = %v", err)
	}
	if value, found, err := getAdapter.Get(ctx, "quote:600000"); !errors.Is(err, getErr) || found || value != nil {
		t.Fatalf("Get() = %q, %v, %v; want nil, false, original error", value, found, err)
	}

	setErr := errors.New("redis SET failed")
	setClient := &fakeGoRedisCommands{setCommand: redis.NewStatusCmd(ctx)}
	setClient.setCommand.SetErr(setErr)
	setAdapter, err := NewGoRedis(setClient)
	if err != nil {
		t.Fatalf("NewGoRedis() SET adapter error = %v", err)
	}
	if err := setAdapter.Set(ctx, "bars:600000", []byte("payload"), time.Second); !errors.Is(err, setErr) {
		t.Fatalf("Set() error = %v, want original error %v", err, setErr)
	}
}

func TestGoRedisPingPropagatesErrors(t *testing.T) {
	ctx := context.Background()
	expected := errors.New("redis unavailable")
	client := &fakeGoRedisCommands{pingCommand: redis.NewStatusCmd(ctx)}
	client.pingCommand.SetErr(expected)
	adapter, err := NewGoRedis(client)
	if err != nil {
		t.Fatalf("NewGoRedis() error = %v", err)
	}
	if err := adapter.Ping(ctx); !errors.Is(err, expected) {
		t.Fatalf("Ping() error = %v, want %v", err, expected)
	}
}

func TestNewGoRedisRejectsNilClient(t *testing.T) {
	if _, err := NewGoRedis(nil); err == nil {
		t.Fatal("NewGoRedis(nil) must return an error")
	}
}
