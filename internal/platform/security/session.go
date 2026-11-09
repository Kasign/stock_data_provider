package security

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

const (
	DefaultSessionIdleTTL     = 12 * time.Hour
	DefaultSessionAbsoluteTTL = 7 * 24 * time.Hour
	SessionTokenBytes         = 32
	maxSessionSubjectBytes    = 128
	maxSessionPrefixBytes     = 128
)

var (
	ErrSessionNotFound = errors.New("session not found or expired")
	ErrSessionConflict = errors.New("session already exists or cannot be rotated")
)

// SessionRecord contains only server-side session metadata. The bearer token
// itself is never part of this value; Redis keys use its SHA-256 digest.
type SessionRecord struct {
	Subject             string    `json:"subject"`
	CreatedAt           time.Time `json:"created_at"`
	LastSeenAt          time.Time `json:"last_seen_at"`
	IdleExpiresAt       time.Time `json:"idle_expires_at"`
	AbsoluteExpiresAt   time.Time `json:"absolute_expires_at"`
	CredentialVersion   uint64    `json:"credential_version"`
	CSRFTokenDigest     string    `json:"csrf_token_digest,omitempty"`
}

// SessionStore accepts token digests only. Implementations must make
// Create/LookupAndTouch/Rotate/Revoke atomic with respect to each session key.
type SessionStore interface {
	Create(ctx context.Context, tokenDigest string, record SessionRecord) error
	LookupAndTouch(ctx context.Context, tokenDigest string, now time.Time, idleTTL time.Duration) (SessionRecord, error)
	Rotate(ctx context.Context, oldTokenDigest, newTokenDigest string, record SessionRecord, now time.Time) error
	Revoke(ctx context.Context, tokenDigest string) error
}

// NewSessionToken creates a 256-bit URL-safe token and its SHA-256 digest.
// The raw token is returned once for the caller to place in an HttpOnly cookie.
func NewSessionToken() (rawToken, tokenDigest string, err error) {
	secret := make([]byte, SessionTokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", "", fault.New(fault.ClassInternal, "platform.security.session_token_generation_failed", "session token could not be generated", err)
	}
	rawToken = base64.RawURLEncoding.EncodeToString(secret)
	digest := sha256.Sum256([]byte(rawToken))
	return rawToken, hex.EncodeToString(digest[:]), nil
}

// SessionTokenDigest validates the token's bounded URL-safe representation and
// returns the digest used as the Redis lookup key.
func SessionTokenDigest(rawToken string) (string, error) {
	if len(rawToken) != base64.RawURLEncoding.EncodedLen(SessionTokenBytes) {
		return "", fault.New(fault.ClassUnauthenticated, "platform.security.invalid_session_token", "session token is invalid", nil)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(rawToken)
	if err != nil || len(decoded) != SessionTokenBytes {
		return "", fault.New(fault.ClassUnauthenticated, "platform.security.invalid_session_token", "session token is invalid", nil)
	}
	digest := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(digest[:]), nil
}

func validSessionDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

func (r SessionRecord) validate() error {
	if strings.TrimSpace(r.Subject) == "" || len(r.Subject) > maxSessionSubjectBytes ||
		r.CredentialVersion == 0 || r.CreatedAt.IsZero() || r.LastSeenAt.IsZero() ||
		r.IdleExpiresAt.IsZero() || r.AbsoluteExpiresAt.IsZero() {
		return errors.New("session record fields are invalid")
	}
	created := r.CreatedAt.UTC()
	lastSeen := r.LastSeenAt.UTC()
	idle := r.IdleExpiresAt.UTC()
	absolute := r.AbsoluteExpiresAt.UTC()
	if lastSeen.Before(created) || idle.After(absolute) || !idle.After(lastSeen) ||
		!absolute.After(created) {
		return errors.New("session record time bounds are invalid")
	}
	if r.CSRFTokenDigest != "" && !validSessionDigest(r.CSRFTokenDigest) {
		return errors.New("CSRF token digest is invalid")
	}
	return nil
}

// RedisSessionStore implements SessionStore on the application's existing
// go-redis client. It never creates or closes a Redis connection pool.
type RedisSessionStore struct {
	client *redis.Client
	prefix string
}

func NewRedisSessionStore(client *redis.Client, prefix string) (*RedisSessionStore, error) {
	prefix = strings.TrimSpace(prefix)
	if client == nil || prefix == "" || len(prefix) > maxSessionPrefixBytes ||
		strings.ContainsAny(prefix, "{} \t\r\n") {
		return nil, fault.New(fault.ClassValidation, "platform.security.invalid_session_store", "session store configuration is invalid", nil)
	}
	return &RedisSessionStore{client: client, prefix: prefix}, nil
}

func (s *RedisSessionStore) key(digest string) string {
	return s.prefix + ":session:" + digest
}

func (s *RedisSessionStore) Create(ctx context.Context, digest string, record SessionRecord) error {
	if s == nil || s.client == nil || ctx == nil || !validSessionDigest(digest) {
		return fault.New(fault.ClassValidation, "platform.security.invalid_session", "session is invalid", nil)
	}
	if err := record.validate(); err != nil {
		return fault.New(fault.ClassValidation, "platform.security.invalid_session", "session is invalid", err)
	}
	now := time.Now().UTC()
	ttl := record.AbsoluteExpiresAt.Sub(now)
	if ttl <= 0 {
		return fault.New(fault.ClassValidation, "platform.security.session_expired", "session is already expired", nil)
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fault.New(fault.ClassInternal, "platform.security.session_encode_failed", "session could not be stored", err)
	}
	err = s.client.SetArgs(ctx, s.key(digest), payload, redis.SetArgs{Mode: "NX", TTL: ttl}).Err()
	if errors.Is(err, redis.Nil) {
		return ErrSessionConflict
	}
	if err != nil {
		return sessionRedisFault("create", err)
	}
	return nil
}

var lookupAndTouchSessionScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then return false end
local record = cjson.decode(raw)
local now = tonumber(ARGV[1])
local idle_ttl = tonumber(ARGV[2])
local last_seen = tonumber(record.last_seen_at_ms or '0')
local idle_exp = tonumber(record.idle_expires_at_ms or '0')
local absolute_exp = tonumber(record.absolute_expires_at_ms or '0')
if absolute_exp <= now or idle_exp <= now then
  redis.call('DEL', KEYS[1])
  return false
end
local next_idle = math.min(now + idle_ttl, absolute_exp)
record.last_seen_at_ms = now
record.idle_expires_at_ms = next_idle
local updated = cjson.encode(record)
redis.call('SET', KEYS[1], updated, 'PX', absolute_exp - now)
return updated
`)

// The stored JSON includes millisecond fields used by Lua. This explicit wire
// shape avoids Lua date parsing and keeps expiry comparisons atomic.
type redisSessionRecord struct {
	Subject           string `json:"subject"`
	CreatedAt         time.Time `json:"created_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	IdleExpiresAt     time.Time `json:"idle_expires_at"`
	AbsoluteExpiresAt time.Time `json:"absolute_expires_at"`
	CredentialVersion uint64    `json:"credential_version"`
	CSRFTokenDigest   string    `json:"csrf_token_digest,omitempty"`
	CreatedAtMS       int64     `json:"created_at_ms"`
	LastSeenAtMS      int64     `json:"last_seen_at_ms"`
	IdleExpiresAtMS   int64     `json:"idle_expires_at_ms"`
	AbsoluteExpiresAtMS int64   `json:"absolute_expires_at_ms"`
}

func toRedisSessionRecord(record SessionRecord) redisSessionRecord {
	return redisSessionRecord{
		Subject: record.Subject, CreatedAt: record.CreatedAt.UTC(),
		LastSeenAt: record.LastSeenAt.UTC(), IdleExpiresAt: record.IdleExpiresAt.UTC(),
		AbsoluteExpiresAt: record.AbsoluteExpiresAt.UTC(), CredentialVersion: record.CredentialVersion,
		CSRFTokenDigest: record.CSRFTokenDigest,
		CreatedAtMS: record.CreatedAt.UnixMilli(), LastSeenAtMS: record.LastSeenAt.UnixMilli(),
		IdleExpiresAtMS: record.IdleExpiresAt.UnixMilli(), AbsoluteExpiresAtMS: record.AbsoluteExpiresAt.UnixMilli(),
	}
}

func fromRedisSessionRecord(record redisSessionRecord) SessionRecord {
	return SessionRecord{
		Subject: record.Subject, CreatedAt: record.CreatedAt.UTC(),
		LastSeenAt: time.UnixMilli(record.CreatedAtMS).UTC(),
		IdleExpiresAt: time.UnixMilli(record.IdleExpiresAtMS).UTC(),
		AbsoluteExpiresAt: time.UnixMilli(record.AbsoluteExpiresAtMS).UTC(),
		CredentialVersion: record.CredentialVersion, CSRFTokenDigest: record.CSRFTokenDigest,
	}
}

func (s *RedisSessionStore) LookupAndTouch(ctx context.Context, digest string, now time.Time, idleTTL time.Duration) (SessionRecord, error) {
	if s == nil || s.client == nil || ctx == nil || !validSessionDigest(digest) ||
		now.IsZero() || idleTTL <= 0 || idleTTL > DefaultSessionAbsoluteTTL {
		return SessionRecord{}, fault.New(fault.ClassValidation, "platform.security.invalid_session_lookup", "session lookup parameters are invalid", nil)
	}
	result, err := lookupAndTouchSessionScript.Run(ctx, s.client, []string{s.key(digest)},
		now.UTC().UnixMilli(), idleTTL.Milliseconds()).Text()
	if errors.Is(err, redis.Nil) {
		return SessionRecord{}, ErrSessionNotFound
	}
	if err != nil {
		return SessionRecord{}, sessionRedisFault("lookup", err)
	}
	var stored redisSessionRecord
	if err := json.Unmarshal([]byte(result), &stored); err != nil {
		return SessionRecord{}, fault.New(fault.ClassInternal, "platform.security.session_decode_failed", "session data is invalid", err)
	}
	record := fromRedisSessionRecord(stored)
	if err := record.validate(); err != nil {
		return SessionRecord{}, fault.New(fault.ClassInternal, "platform.security.session_corrupt", "session data is invalid", err)
	}
	return record, nil
}

var rotateSessionScript = redis.NewScript(`
local old_raw = redis.call('GET', KEYS[1])
if not old_raw then return 0 end
local old = cjson.decode(old_raw)
local now = tonumber(ARGV[1])
local old_idle = tonumber(old.idle_expires_at_ms or '0')
local old_absolute = tonumber(old.absolute_expires_at_ms or '0')
if old_idle <= now or old_absolute <= now then
  redis.call('DEL', KEYS[1])
  return 0
end
if redis.call('EXISTS', KEYS[2]) == 1 then return -1 end
local ttl = tonumber(ARGV[2]) - now
if ttl <= 0 then return 0 end
redis.call('SET', KEYS[2], ARGV[3], 'PX', ttl, 'NX')
redis.call('DEL', KEYS[1])
return 1
`)

func (s *RedisSessionStore) Rotate(ctx context.Context, oldDigest, newDigest string, record SessionRecord, now time.Time) error {
	if s == nil || s.client == nil || ctx == nil || !validSessionDigest(oldDigest) ||
		!validSessionDigest(newDigest) || oldDigest == newDigest || now.IsZero() {
		return fault.New(fault.ClassValidation, "platform.security.invalid_session_rotation", "session rotation parameters are invalid", nil)
	}
	if err := record.validate(); err != nil {
		return fault.New(fault.ClassValidation, "platform.security.invalid_session", "session is invalid", err)
	}
	if record.CreatedAt.After(now) || !record.AbsoluteExpiresAt.After(now) {
		return fault.New(fault.ClassValidation, "platform.security.invalid_session", "rotated session time bounds are invalid", nil)
	}
	payload, err := json.Marshal(toRedisSessionRecord(record))
	if err != nil {
		return fault.New(fault.ClassInternal, "platform.security.session_encode_failed", "session could not be stored", err)
	}
	result, err := rotateSessionScript.Run(ctx, s.client, []string{s.key(oldDigest), s.key(newDigest)},
		now.UTC().UnixMilli(), record.AbsoluteExpiresAt.UnixMilli(), payload).Int()
	if err != nil {
		return sessionRedisFault("rotate", err)
	}
	switch result {
	case 1:
		return nil
	case -1:
		return ErrSessionConflict
	default:
		return ErrSessionNotFound
	}
}

func (s *RedisSessionStore) Revoke(ctx context.Context, digest string) error {
	if s == nil || s.client == nil || ctx == nil || !validSessionDigest(digest) {
		return fault.New(fault.ClassValidation, "platform.security.invalid_session", "session is invalid", nil)
	}
	if err := s.client.Del(ctx, s.key(digest)).Err(); err != nil {
		return sessionRedisFault("revoke", err)
	}
	return nil
}

func sessionRedisFault(operation string, cause error) *fault.Fault {
	return &fault.Fault{
		Class: fault.ClassUnavailable, Code: "platform.security.session_store_unavailable",
		SafeMessage: "session store is unavailable", Dependency: "redis",
		Retryable: false, Cause: fmt.Errorf("session %s: %w", operation, cause),
	}
}
