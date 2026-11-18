package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	"example.com/stock-analysis-flow/internal/server/resources"
)

func TestOwnerSessionHTTPMariaDBRedisLifecycleIntegration(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to run Owner Session MariaDB/Redis integration")
	}
	baseDSN := strings.TrimSpace(os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN"))
	redisAddr := strings.TrimSpace(os.Getenv("PLATFORM_REDIS_ADDR"))
	if baseDSN == "" || redisAddr == "" {
		t.Skip("Integration CI must provide MariaDB DSN and Redis address")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	mysqlConfig, err := mysql.ParseDSN(baseDSN)
	if err != nil {
		t.Fatalf("parse MariaDB DSN: %v", err)
	}
	admin, err := sql.Open("mysql", baseDSN)
	if err != nil {
		t.Fatalf("open MariaDB admin connection: %v", err)
	}
	defer admin.Close()

	databaseName := fmt.Sprintf("owner_session_it_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+databaseName+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create isolated integration database: %v", err)
	}
	defer admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+databaseName)

	mysqlConfig.DBName = databaseName
	db, err := sql.Open("mysql", mysqlConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open isolated MariaDB database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping isolated MariaDB database: %v", err)
	}

	prefix := fmt.Sprintf("owner-session-it-%d", time.Now().UnixNano())
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer redisClient.Close()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping integration Redis: %v", err)
	}
	defer func() {
		var cursor uint64
		for {
			keys, next, scanErr := redisClient.Scan(context.Background(), cursor, prefix+"*", 100).Result()
			if scanErr != nil {
				break
			}
			if len(keys) > 0 {
				_ = redisClient.Del(context.Background(), keys...).Err()
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}()

	registry, err := newMigrationRegistry()
	if err != nil {
		t.Fatalf("create Server migration registry: %v", err)
	}
	migrationCtx, migrationCancel := context.WithTimeout(ctx, 45*time.Second)
	_, err = registry.RunWithReport(migrationCtx, db)
	migrationCancel()
	if err != nil {
		t.Fatalf("apply isolated Server migrations: %v", err)
	}

	cfg := ownerSessionTestConfig()
	cfg.RedisKeyPrefix = prefix
	composition, err := newOwnerSessionComposition(&resources.Resources{DB: db, Redis: redisClient}, cfg)
	if err != nil {
		t.Fatalf("compose production Owner Session dependencies: %v", err)
	}
	if err := composition.lifecycle.InitializeOwner(ctx, []byte("initial-owner-password"), "req-owner-init-it"); err != nil {
		t.Fatalf("initialize Owner Credential: %v", err)
	}

	bearerToken := strings.Repeat("b", 43)
	bearerSecret := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "PLATFORM_ADMIN_TOKEN" {
			return bearerToken, true
		}
		return "", false
	}).Secret("PLATFORM_ADMIN_TOKEN")
	bearer, err := platformsecurity.NewAuthenticator(bearerSecret, nil)
	if err != nil {
		t.Fatalf("create Owner Bearer authenticator: %v", err)
	}
	health := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	legacy := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	marketData := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler, err := newHTTPHandler(bearer, health, legacy, marketData, composition)
	if err != nil {
		t.Fatalf("mount Root Server routes: %v", err)
	}
	handler, err = newHTTPMiddleware(handler, httpMiddlewareConfig{maxRequestBodyBytes: 64 * 1024, maxConcurrentRequests: 8})
	if err != nil {
		t.Fatalf("apply HTTP middleware: %v", err)
	}

	login := func(password string) (*http.Cookie, string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "https://stock.example.test/api/v1/owner/session",
			strings.NewReader(fmt.Sprintf("{\"password\":%q}", password)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "https://stock.example.test")
		request.RemoteAddr = "192.0.2.10:4321"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("Owner login status=%d body=%s", response.Code, response.Body.String())
		}
		cookies := response.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure {
			t.Fatalf("Owner login did not issue secure HttpOnly cookie: %+v", cookies)
		}
		var body struct {
			CSRFToken string `json:"csrf_token"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.CSRFToken == "" {
			t.Fatalf("Owner login did not return CSRF token: %v", err)
		}
		return cookies[0], body.CSRFToken
	}

	cookie, csrfToken := login("initial-owner-password")
	current := httptest.NewRequest(http.MethodGet, "https://stock.example.test/api/v1/owner/session", nil)
	current.AddCookie(cookie)
	currentResponse := httptest.NewRecorder()
	handler.ServeHTTP(currentResponse, current)
	if currentResponse.Code != http.StatusOK {
		t.Fatalf("current Owner Session status=%d body=%s", currentResponse.Code, currentResponse.Body.String())
	}

	refresh := httptest.NewRequest(http.MethodPost, "https://stock.example.test/api/v1/owner/session/csrf", nil)
	refresh.AddCookie(cookie)
	refresh.Header.Set("Origin", "https://stock.example.test")
	refreshResponse := httptest.NewRecorder()
	handler.ServeHTTP(refreshResponse, refresh)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("CSRF recovery status=%d body=%s", refreshResponse.Code, refreshResponse.Body.String())
	}
	var refreshed struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(refreshResponse.Body.Bytes(), &refreshed); err != nil || refreshed.CSRFToken == "" || refreshed.CSRFToken == csrfToken {
		t.Fatalf("CSRF recovery did not rotate token: %v", err)
	}

	rotate := httptest.NewRequest(http.MethodPost, "https://stock.example.test/api/v1/owner/session/rotate", nil)
	rotate.AddCookie(cookie)
	rotate.Header.Set("Origin", "https://stock.example.test")
	rotate.Header.Set("X-CSRF-Token", refreshed.CSRFToken)
	rotateResponse := httptest.NewRecorder()
	handler.ServeHTTP(rotateResponse, rotate)
	if rotateResponse.Code != http.StatusOK {
		t.Fatalf("session rotation status=%d body=%s", rotateResponse.Code, rotateResponse.Body.String())
	}
	rotatedCookies := rotateResponse.Result().Cookies()
	if len(rotatedCookies) != 1 || rotatedCookies[0].Value == cookie.Value {
		t.Fatalf("session rotation did not replace cookie: %+v", rotatedCookies)
	}
	var rotatedBody struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rotateResponse.Body.Bytes(), &rotatedBody); err != nil || rotatedBody.CSRFToken == "" {
		t.Fatalf("session rotation did not return CSRF token: %v", err)
	}

	change := httptest.NewRequest(http.MethodPut, "https://stock.example.test/api/v1/owner/password",
		strings.NewReader(`{"current_password":"initial-owner-password","new_password":"updated-owner-password"}`))
	change.AddCookie(rotatedCookies[0])
	change.Header.Set("Content-Type", "application/json")
	change.Header.Set("Origin", "https://stock.example.test")
	change.Header.Set("X-CSRF-Token", rotatedBody.CSRFToken)
	changeResponse := httptest.NewRecorder()
	handler.ServeHTTP(changeResponse, change)
	if changeResponse.Code != http.StatusNoContent {
		t.Fatalf("Owner password change status=%d body=%s", changeResponse.Code, changeResponse.Body.String())
	}

	stale := httptest.NewRequest(http.MethodGet, "https://stock.example.test/api/v1/owner/session", nil)
	stale.AddCookie(rotatedCookies[0])
	staleResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleResponse, stale)
	if staleResponse.Code != http.StatusUnauthorized {
		t.Fatalf("old CredentialVersion session status=%d body=%s", staleResponse.Code, staleResponse.Body.String())
	}

	newCookie, newCSRF := login("updated-owner-password")
	logout := httptest.NewRequest(http.MethodDelete, "https://stock.example.test/api/v1/owner/session", nil)
	logout.AddCookie(newCookie)
	logout.Header.Set("Origin", "https://stock.example.test")
	logout.Header.Set("X-CSRF-Token", newCSRF)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("Owner logout status=%d body=%s", logoutResponse.Code, logoutResponse.Body.String())
	}
	if len(logoutResponse.Result().Cookies()) != 1 || logoutResponse.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatal("Owner logout did not clear the browser cookie")
	}

	auditCount := 0
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM server_security_audit_events").Scan(&auditCount); err != nil {
		t.Fatalf("read security audit evidence: %v", err)
	}
	if auditCount < 8 {
		t.Fatalf("security audit events=%d, want at least 8 lifecycle events", auditCount)
	}
}
