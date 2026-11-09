package security

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

const (
	maxRateLimitWindow   = 24 * time.Hour
	maxRateLimitBackoff  = 24 * time.Hour
	maxRateLimitFailures = 100000
)

// LoginRateLimitPolicy controls fixed-window failure counters and exponential
// temporary backoff. Account and source-IP limits are evaluated together.
type LoginRateLimitPolicy struct {
	AccountMaxFailures int64
	IPMaxFailures      int64
	Window             time.Duration
	BaseBackoff        time.Duration
	MaxBackoff         time.Duration
}

func (p LoginRateLimitPolicy) validate() error {
	if p.AccountMaxFailures < 1 || p.AccountMaxFailures > maxRateLimitFailures ||
		p.IPMaxFailures < 1 || p.IPMaxFailures > maxRateLimitFailures ||
		p.Window < time.Millisecond || p.Window > maxRateLimitWindow ||
		p.BaseBackoff < time.Millisecond || p.BaseBackoff > maxRateLimitBackoff ||
		p.MaxBackoff < p.BaseBackoff || p.MaxBackoff > maxRateLimitBackoff {
		return errors.New("login rate limit policy is invalid")
	}
	return nil
}

// LoginRateLimitResult describes the state after a check or recorded failure.
type LoginRateLimitResult struct {
	Allowed         bool
	AccountFailures int64
	IPFailures      int64
	RetryAfter      time.Duration
}

// LoginRateLimiter checks active blocks and atomically records failed attempts.
// Identifiers are pre-hashed by the caller; raw account/IP values are never keys.
type LoginRateLimiter interface {
	Check(ctx context.Context, accountDigest, sourceIPDigest string) (LoginRateLimitResult, error)
	RecordFailure(ctx context.Context, accountDigest, sourceIPDigest string, policy LoginRateLimitPolicy) (LoginRateLimitResult, error)
	ResetAccount(ctx context.Context, accountDigest string) error
}

// RedisLoginRateLimiter uses the application's existing Redis client. The
// caller owns the client lifecycle; this type never opens or closes a pool.
type RedisLoginRateLimiter struct {
	client *redis.Client
	prefix string
}

func NewRedisLoginRateLimiter(client *redis.Client, prefix string) (*RedisLoginRateLimiter, error) {
	prefix = strings.TrimSpace(prefix)
	if client == nil || prefix == "" || len(prefix) > maxSessionPrefixBytes ||
		strings.ContainsAny(prefix, "{} \t\r\n") {
		return nil, fault.New(fault.ClassValidation, "platform.security.invalid_rate_limiter", "rate limiter configuration is invalid", nil)
	}
	return &RedisLoginRateLimiter{client: client, prefix: prefix}, nil
}

func (r *RedisLoginRateLimiter) accountKeys(digest string) (string, string) {
	base := r.prefix + ":login:account:" + digest
	return base + ":failures", base + ":blocked"
}

func (r *RedisLoginRateLimiter) ipKeys(digest string) (string, string) {
	base := r.prefix + ":login:ip:" + digest
	return base + ":failures", base + ":blocked"
}

var checkLoginRateLimitScript = redis.NewScript(`
local account_count = tonumber(redis.call('GET', KEYS[1]) or '0')
local ip_count = tonumber(redis.call('GET', KEYS[3]) or '0')
local account_ttl = redis.call('PTTL', KEYS[2])
local ip_ttl = redis.call('PTTL', KEYS[4])
if account_ttl == -2 then account_ttl = 0 end
if ip_ttl == -2 then ip_ttl = 0 end
return {account_count, ip_count, account_ttl, ip_ttl}
`)

var recordLoginFailureScript = redis.NewScript(`
local account_count = redis.call('INCR', KEYS[1])
if account_count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
local ip_count = redis.call('INCR', KEYS[3])
if ip_count == 1 then redis.call('PEXPIRE', KEYS[3], ARGV[1]) end
local account_limit = tonumber(ARGV[2])
local ip_limit = tonumber(ARGV[3])
local base_delay = tonumber(ARGV[4])
local max_delay = tonumber(ARGV[5])
local function apply_backoff(count, limit, block_key)
  if count > limit then
    local delay = base_delay
    local steps = count - limit - 1
    while steps > 0 and delay < max_delay do
      delay = math.min(delay * 2, max_delay)
      steps = steps - 1
    end
    local current = redis.call('PTTL', block_key)
    if current < delay then redis.call('PSETEX', block_key, delay, '1') end
  end
  local ttl = redis.call('PTTL', block_key)
  if ttl == -2 then return 0 end
  return ttl
end
local account_ttl = apply_backoff(account_count, account_limit, KEYS[2])
local ip_ttl = apply_backoff(ip_count, ip_limit, KEYS[4])
return {account_count, ip_count, account_ttl, ip_ttl}
`)

func (r *RedisLoginRateLimiter) Check(ctx context.Context, accountDigest, sourceIPDigest string) (LoginRateLimitResult, error) {
	if err := r.validateKeys(ctx, accountDigest, sourceIPDigest); err != nil {
		return LoginRateLimitResult{}, err
	}
	accountCounter, accountBlock := r.accountKeys(accountDigest)
	ipCounter, ipBlock := r.ipKeys(sourceIPDigest)
	values, err := checkLoginRateLimitScript.Run(ctx, r.client, []string{accountCounter, accountBlock, ipCounter, ipBlock}).Slice()
	if err != nil {
		return LoginRateLimitResult{}, rateLimitRedisFault("check", err)
	}
	return parseLoginRateLimitResult(values)
}

func (r *RedisLoginRateLimiter) RecordFailure(ctx context.Context, accountDigest, sourceIPDigest string, policy LoginRateLimitPolicy) (LoginRateLimitResult, error) {
	if err := r.validateKeys(ctx, accountDigest, sourceIPDigest); err != nil {
		return LoginRateLimitResult{}, err
	}
	if err := policy.validate(); err != nil {
		return LoginRateLimitResult{}, fault.New(fault.ClassValidation, "platform.security.invalid_rate_limit_policy", "rate limit policy is invalid", err)
	}
	accountCounter, accountBlock := r.accountKeys(accountDigest)
	ipCounter, ipBlock := r.ipKeys(sourceIPDigest)
	values, err := recordLoginFailureScript.Run(ctx, r.client, []string{accountCounter, accountBlock, ipCounter, ipBlock},
		policy.Window.Milliseconds(), policy.AccountMaxFailures, policy.IPMaxFailures,
		policy.BaseBackoff.Milliseconds(), policy.MaxBackoff.Milliseconds()).Slice()
	if err != nil {
		return LoginRateLimitResult{}, rateLimitRedisFault("record failure", err)
	}
	return parseLoginRateLimitResult(values)
}

func (r *RedisLoginRateLimiter) ResetAccount(ctx context.Context, accountDigest string) error {
	if r == nil || r.client == nil || ctx == nil || !validSessionDigest(accountDigest) {
		return fault.New(fault.ClassValidation, "platform.security.invalid_rate_limit_key", "rate limit key is invalid", nil)
	}
	counter, block := r.accountKeys(accountDigest)
	if err := r.client.Del(ctx, counter, block).Err(); err != nil {
		return rateLimitRedisFault("reset account", err)
	}
	return nil
}

func (r *RedisLoginRateLimiter) validateKeys(ctx context.Context, accountDigest, sourceIPDigest string) error {
	if r == nil || r.client == nil || ctx == nil ||
		!validSessionDigest(accountDigest) || !validSessionDigest(sourceIPDigest) {
		return fault.New(fault.ClassValidation, "platform.security.invalid_rate_limit_key", "rate limit key is invalid", nil)
	}
	return nil
}

func parseLoginRateLimitResult(values []interface{}) (LoginRateLimitResult, error) {
	if len(values) != 4 {
		return LoginRateLimitResult{}, fault.New(fault.ClassInternal, "platform.security.invalid_rate_limit_state", "rate limit state is invalid", nil)
	}
	numbers := make([]int64, 4)
	for i, value := range values {
		number, ok := value.(int64)
		if !ok {
			return LoginRateLimitResult{}, fault.New(fault.ClassInternal, "platform.security.invalid_rate_limit_state", "rate limit state is invalid", nil)
		}
		numbers[i] = number
	}
	if numbers[0] < 0 || numbers[1] < 0 || numbers[2] < -2 || numbers[3] < -2 {
		return LoginRateLimitResult{}, fault.New(fault.ClassInternal, "platform.security.invalid_rate_limit_state", "rate limit state is invalid", nil)
	}
	if numbers[2] == -1 || numbers[3] == -1 {
		return LoginRateLimitResult{}, fault.New(fault.ClassUnavailable, "platform.security.rate_limit_state_invalid", "rate limit state is invalid", nil)
	}
	retryMillis := numbers[2]
	if numbers[3] > retryMillis {
		retryMillis = numbers[3]
	}
	return LoginRateLimitResult{
		Allowed: retryMillis == 0, AccountFailures: numbers[0], IPFailures: numbers[1],
		RetryAfter: time.Duration(retryMillis) * time.Millisecond,
	}, nil
}

func rateLimitRedisFault(operation string, cause error) *fault.Fault {
	return &fault.Fault{
		Class: fault.ClassUnavailable, Code: "platform.security.rate_limiter_unavailable",
		SafeMessage: "login rate limiter is unavailable", Dependency: "redis",
		Retryable: false, Cause: fmt.Errorf("rate limiter %s: %w", operation, cause),
	}
}
