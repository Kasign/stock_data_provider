package bootstrap

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"
	"net/http"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

func TestNewOwnerSessionCompositionDisabled(t *testing.T) {
	composition, err := newOwnerSessionComposition(nil, serverconfig.OwnerSession{})
	if err != nil {
		t.Fatalf("disabled composition returned error: %v", err)
	}
	if composition != nil {
		t.Fatal("disabled composition must not allocate dependencies")
	}
}

func TestNewOwnerSessionCompositionRejectsMissingSharedResources(t *testing.T) {
	cfg := ownerSessionTestConfig()
	tests := []struct {
		name string
		resources *serverresources.Resources
	}{
		{name: "nil resources"},
		{name: "missing MariaDB", resources: &serverresources.Resources{Redis: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})}},
		{name: "missing Redis", resources: &serverresources.Resources{DB: &sql.DB{}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.resources != nil && test.resources.Redis != nil {
				t.Cleanup(func() { _ = test.resources.Redis.Close() })
			}
			if composition, err := newOwnerSessionComposition(test.resources, cfg); err == nil || composition != nil {
				t.Fatalf("composition = %v, error = %v; want fail-closed error", composition, err)
			}
		})
	}
}

func TestNewOwnerSessionCompositionRejectsInvalidHasherConfig(t *testing.T) {
	db := &sql.DB{}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer redisClient.Close()
	cfg := ownerSessionTestConfig()
	cfg.Argon2id = platformsecurity.Argon2idParams{}
	if composition, err := newOwnerSessionComposition(&serverresources.Resources{DB: db, Redis: redisClient}, cfg); err == nil || composition != nil {
		t.Fatalf("composition = %v, error = %v; want invalid Argon2id error", composition, err)
	}
}

func TestNewOwnerSessionCompositionBuildsProductionDependencies(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN"))
	redisAddr := strings.TrimSpace(os.Getenv("PLATFORM_REDIS_ADDR"))
	if dsn == "" || redisAddr == "" {
		t.Skip("需要 Integration CI 提供 MariaDB DSN 与 Redis 地址")
	}
	if _, err := mysql.ParseDSN(dsn); err != nil {
		t.Fatalf("解析 MariaDB DSN 失败: %v", err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("打开 MariaDB 连接失败: %v", err)
	}
	defer db.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer redisClient.Close()

	composition, err := newOwnerSessionComposition(
		&serverresources.Resources{DB: db, Redis: redisClient},
		ownerSessionTestConfig(),
	)
	if err != nil {
		t.Fatalf("构造 Owner Session 依赖失败: %v", err)
	}
	if composition == nil || composition.lifecycle == nil || composition.authenticator == nil {
		t.Fatal("Owner Session lifecycle/authenticator 未完整构造")
	}
}

func ownerSessionTestConfig() serverconfig.OwnerSession {
	return serverconfig.OwnerSession{
		Enabled: true,
		PublicOrigin: "https://stock.example.test",
		Cookie: platformsecurity.SessionCookieConfig{
			Name: "__Secure-owner_session", Path: "/api", Secure: true, SameSite: http.SameSiteLaxMode,
		},
		CSRFHeader: "X-CSRF-Token",
		IdleTTL: 12 * time.Hour,
		AbsoluteTTL: 7 * 24 * time.Hour,
		RedisKeyPrefix: "owner-session-composition-test",
		Argon2id: platformsecurity.Argon2idParams{
			Memory: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
		},
		RateLimitPolicy: platformsecurity.LoginRateLimitPolicy{
			AccountMaxFailures: 5, IPMaxFailures: 10, Window: 15 * time.Minute,
			BaseBackoff: time.Second, MaxBackoff: 15 * time.Minute,
		},
	}
}

