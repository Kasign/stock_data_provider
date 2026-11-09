package security

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

func TestLoginRateLimitPolicyValidation(t *testing.T) {
	valid := LoginRateLimitPolicy{
		AccountMaxFailures: 2, IPMaxFailures: 3, Window: time.Hour,
		BaseBackoff: 100 * time.Millisecond, MaxBackoff: time.Minute,
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	valid.MaxBackoff = valid.BaseBackoff / 2
	if err := valid.validate(); err == nil {
		t.Fatal("max backoff below base backoff must be rejected")
	}
}

func TestRedisLoginRateLimiterIntegration(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to run real Redis integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("PLATFORM_REDIS_ADDR")})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Fatalf("connect to Redis: %v", err)
	}
	defer client.Close()
	prefix := "platform:rate-limit-it:" + time.Now().Format("20060102150405.000000000")
	limiter, err := NewRedisLoginRateLimiter(client, prefix)
	if err != nil {
		t.Fatal(err)
	}
	accountDigest := strings.Repeat("a", 64)
	ipDigest := strings.Repeat("b", 64)
	policy := LoginRateLimitPolicy{
		AccountMaxFailures: 1, IPMaxFailures: 2, Window: time.Minute,
		BaseBackoff: 300 * time.Millisecond, MaxBackoff: 1200 * time.Millisecond,
	}
	initial, err := limiter.Check(ctx, accountDigest, ipDigest)
	if err != nil || !initial.Allowed {
		t.Fatalf("initial check = %+v, %v", initial, err)
	}
	first, err := limiter.RecordFailure(ctx, accountDigest, ipDigest, policy)
	if err != nil || !first.Allowed || first.AccountFailures != 1 || first.IPFailures != 1 {
		t.Fatalf("first failure = %+v, %v", first, err)
	}
	second, err := limiter.RecordFailure(ctx, accountDigest, ipDigest, policy)
	if err != nil || second.Allowed || second.AccountFailures != 2 || second.IPFailures != 2 || second.RetryAfter <= 0 {
		t.Fatalf("second failure should trigger account backoff: %+v, %v", second, err)
	}
	blocked, err := limiter.Check(ctx, accountDigest, ipDigest)
	if err != nil || blocked.Allowed || blocked.RetryAfter <= 0 {
		t.Fatalf("check after account block = %+v, %v", blocked, err)
	}
	time.Sleep(second.RetryAfter + 20*time.Millisecond)
	third, err := limiter.RecordFailure(ctx, accountDigest, ipDigest, policy)
	if err != nil || third.Allowed || third.AccountFailures != 3 || third.IPFailures != 3 || third.RetryAfter <= 0 {
		t.Fatalf("third failure should apply account and IP backoff: %+v, %v", third, err)
	}
	if err := limiter.ResetAccount(ctx, accountDigest); err != nil {
		t.Fatalf("reset account failures: %v", err)
	}
	stillBlocked, err := limiter.Check(ctx, accountDigest, ipDigest)
	if err != nil || stillBlocked.Allowed {
		t.Fatalf("resetting account must not clear source-IP block: %+v, %v", stillBlocked, err)
	}
	time.Sleep(stillBlocked.RetryAfter + 20*time.Millisecond)
	cleared, err := limiter.Check(ctx, accountDigest, ipDigest)
	if err != nil || !cleared.Allowed || cleared.AccountFailures != 0 {
		t.Fatalf("rate limit should expire and account reset should persist: %+v, %v", cleared, err)
	}
	var wait sync.WaitGroup
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := limiter.RecordFailure(ctx, accountDigest, ipDigest, policy); err != nil {
				t.Errorf("concurrent failure record: %v", err)
			}
		}()
	}
	wait.Wait()
	concurrent, err := limiter.Check(ctx, accountDigest, ipDigest)
	if err != nil || concurrent.AccountFailures != 8 || concurrent.IPFailures != 8 || concurrent.Allowed {
		t.Fatalf("concurrent counters are not atomic: %+v, %v", concurrent, err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close Redis client: %v", err)
	}
	if _, err := limiter.Check(ctx, accountDigest, ipDigest); err == nil {
		t.Fatal("Redis outage must fail closed")
	} else {
		var storeFault *fault.Fault
		if !errors.As(err, &storeFault) || storeFault.Dependency != "redis" {
			t.Fatalf("Redis outage returned unexpected error: %v", err)
		}
	}
}
