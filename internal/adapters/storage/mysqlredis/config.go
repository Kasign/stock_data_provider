package mysqlredis

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DBHost            string
	DBPort            int
	DBUser            string
	DBPassword        string
	DBName            string
	DBParams          string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	DBAutoMigrate     bool
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	RedisKeyPrefix    string
	RedisCacheTTL     time.Duration
	RedisDialTimeout  time.Duration
	RedisReadTimeout  time.Duration
	RedisWriteTimeout time.Duration
}

func LoadConfigFromEnv() (Config, error) {
	c := Config{
		DBHost: envString("DB_HOST", "127.0.0.1"), DBUser: envString("DB_USER", "stock_analysis"),
		DBPassword: os.Getenv("DB_PASSWORD"), DBName: envString("DB_NAME", "stock_analysis"),
		DBParams:  envString("DB_PARAMS", "charset=utf8mb4&parseTime=true&loc=UTC"),
		RedisAddr: envString("REDIS_ADDR", "127.0.0.1:6379"), RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisKeyPrefix: envString("REDIS_KEY_PREFIX", "stock-analysis:"),
	}
	var err error
	if c.DBPort, err = envInt("DB_PORT", 3306); err != nil {
		return c, err
	}
	if c.DBMaxOpenConns, err = envInt("DB_MAX_OPEN_CONNS", 20); err != nil {
		return c, err
	}
	if c.DBMaxIdleConns, err = envInt("DB_MAX_IDLE_CONNS", 5); err != nil {
		return c, err
	}
	if c.RedisDB, err = envInt("REDIS_DB", 0); err != nil {
		return c, err
	}
	if c.DBAutoMigrate, err = envBool("DB_AUTO_MIGRATE", false); err != nil {
		return c, err
	}
	if c.DBConnMaxLifetime, err = envDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute); err != nil {
		return c, err
	}
	if c.RedisCacheTTL, err = envDuration("REDIS_CACHE_TTL", 5*time.Minute); err != nil {
		return c, err
	}
	if c.RedisDialTimeout, err = envDuration("REDIS_DIAL_TIMEOUT", 3*time.Second); err != nil {
		return c, err
	}
	if c.RedisReadTimeout, err = envDuration("REDIS_READ_TIMEOUT", 2*time.Second); err != nil {
		return c, err
	}
	if c.RedisWriteTimeout, err = envDuration("REDIS_WRITE_TIMEOUT", 2*time.Second); err != nil {
		return c, err
	}
	if strings.TrimSpace(c.DBHost) == "" || strings.TrimSpace(c.DBUser) == "" || strings.TrimSpace(c.DBName) == "" {
		return c, fmt.Errorf("DB_HOST, DB_USER and DB_NAME must not be empty")
	}
	if c.DBPort < 1 || c.DBPort > 65535 || c.DBMaxOpenConns < 1 || c.DBMaxIdleConns < 0 || c.RedisDB < 0 {
		return c, fmt.Errorf("invalid database or Redis numeric configuration")
	}
	if _, _, err := net.SplitHostPort(c.RedisAddr); err != nil {
		return c, fmt.Errorf("REDIS_ADDR must be host:port: %w", err)
	}
	return c, nil
}

func (c Config) MySQLDSN() string {
	params := strings.TrimPrefix(c.DBParams, "?")
	if params == "" {
		params = "charset=utf8mb4&parseTime=true&loc=UTC"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s", c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, params)
}

func (c Config) RedisKey(key string) string { return c.RedisKeyPrefix + strings.TrimLeft(key, ":") }

func envString(k, fallback string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return fallback
}
func envInt(k string, fallback int) (int, error) {
	v, ok := os.LookupEnv(k)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	n, e := strconv.Atoi(v)
	if e != nil {
		return 0, fmt.Errorf("%s: %w", k, e)
	}
	return n, nil
}
func envBool(k string, fallback bool) (bool, error) {
	v, ok := os.LookupEnv(k)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	b, e := strconv.ParseBool(v)
	if e != nil {
		return false, fmt.Errorf("%s: %w", k, e)
	}
	return b, nil
}
func envDuration(k string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(k)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	d, e := time.ParseDuration(v)
	if e != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", k)
	}
	return d, nil
}

// EscapePassword is provided for diagnostics only; do not log DSNs or credentials.
func EscapePassword(s string) string { return url.QueryEscape(s) }
