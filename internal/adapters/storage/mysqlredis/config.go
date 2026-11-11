package mysqlredis

import (
	"database/sql/driver"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
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

// LoadConfig decodes the storage adapter's configuration from an explicit source.
func LoadConfig(env platformconfig.Env) (Config, error) {
	c := Config{
		DBHost: env.String("DB_HOST", "127.0.0.1"), DBUser: env.String("DB_USER", "stock_analysis"),
		DBPassword: env.Secret("DB_PASSWORD").Value(), DBName: env.String("DB_NAME", "stock_analysis"),
		DBParams:  env.String("DB_PARAMS", "charset=utf8mb4&parseTime=true&loc=UTC"),
		RedisAddr: env.String("REDIS_ADDR", "127.0.0.1:6379"), RedisPassword: env.Secret("REDIS_PASSWORD").Value(),
		RedisKeyPrefix: env.String("REDIS_KEY_PREFIX", "stock-analysis:"),
	}
	var err error
	if c.DBPort, err = env.Int("DB_PORT", 3306); err != nil {
		return c, err
	}
	if c.DBMaxOpenConns, err = env.Int("DB_MAX_OPEN_CONNS", 20); err != nil {
		return c, err
	}
	if c.DBMaxIdleConns, err = env.Int("DB_MAX_IDLE_CONNS", 5); err != nil {
		return c, err
	}
	if c.RedisDB, err = env.Int("REDIS_DB", 0); err != nil {
		return c, err
	}
	if c.DBAutoMigrate, err = env.Bool("DB_AUTO_MIGRATE", false); err != nil {
		return c, err
	}
	if c.DBConnMaxLifetime, err = env.Duration("DB_CONN_MAX_LIFETIME", 30*time.Minute); err != nil {
		return c, err
	}
	if c.RedisCacheTTL, err = env.Duration("REDIS_CACHE_TTL", 5*time.Minute); err != nil {
		return c, err
	}
	if c.RedisDialTimeout, err = env.Duration("REDIS_DIAL_TIMEOUT", 3*time.Second); err != nil {
		return c, err
	}
	if c.RedisReadTimeout, err = env.Duration("REDIS_READ_TIMEOUT", 2*time.Second); err != nil {
		return c, err
	}
	if c.RedisWriteTimeout, err = env.Duration("REDIS_WRITE_TIMEOUT", 2*time.Second); err != nil {
		return c, err
	}
	if c.DBConnMaxLifetime <= 0 || c.RedisCacheTTL <= 0 || c.RedisDialTimeout <= 0 || c.RedisReadTimeout <= 0 || c.RedisWriteTimeout <= 0 {
		return c, fmt.Errorf("database and Redis durations must be positive")
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

// LoadConfigFromEnv preserves the legacy adapter entry point.
func LoadConfigFromEnv() (Config, error) {
	return LoadConfig(platformconfig.NewEnv())
}

// mysqlDriverConfig builds a driver-native config. Keeping credentials as fields
// avoids parsing a DSN string when passwords contain reserved characters.
func (c Config) mysqlDriverConfig() (*mysqlDriver.Config, error) {
	if strings.TrimSpace(c.DBHost) == "" || strings.TrimSpace(c.DBUser) == "" || strings.TrimSpace(c.DBName) == "" {
		return nil, fmt.Errorf("DB_HOST, DB_USER and DB_NAME must not be empty")
	}
	if c.DBPort < 1 || c.DBPort > 65535 {
		return nil, fmt.Errorf("DB_PORT must be between 1 and 65535")
	}

	rawParams := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c.DBParams), "?"))
	if rawParams == "" {
		rawParams = "charset=utf8mb4&parseTime=true&loc=UTC"
	}
	values, err := url.ParseQuery(rawParams)
	if err != nil {
		return nil, fmt.Errorf("DB_PARAMS must be valid query parameters: %w", err)
	}

	driverConfig := mysqlDriver.NewConfig()
	driverConfig.User = c.DBUser
	driverConfig.Passwd = c.DBPassword
	driverConfig.Net = "tcp"
	driverConfig.Addr = net.JoinHostPort(c.DBHost, strconv.Itoa(c.DBPort))
	driverConfig.DBName = c.DBName
	driverConfig.Params = make(map[string]string, len(values))

	if raw, ok := values["parseTime"]; ok {
		if len(raw) != 1 {
			return nil, fmt.Errorf("DB_PARAMS parseTime must be specified once")
		}
		parseTime, err := strconv.ParseBool(raw[0])
		if err != nil {
			return nil, fmt.Errorf("DB_PARAMS parseTime must be true or false")
		}
		driverConfig.ParseTime = parseTime
		delete(values, "parseTime")
	}
	if raw, ok := values["loc"]; ok {
		if len(raw) != 1 {
			return nil, fmt.Errorf("DB_PARAMS loc must be specified once")
		}
		location, err := time.LoadLocation(raw[0])
		if err != nil {
			return nil, fmt.Errorf("DB_PARAMS loc must be a valid IANA time zone: %w", err)
		}
		driverConfig.Loc = location
		delete(values, "loc")
	}
	if raw, ok := values["charset"]; ok {
		if len(raw) != 1 || strings.TrimSpace(raw[0]) == "" {
			return nil, fmt.Errorf("DB_PARAMS charset must be specified once and must not be empty")
		}
		// charset is a driver option, not a MariaDB session variable. Putting it
		// in Config.Params makes the driver execute SET charset = 'utf8mb4',
		// which fails with MariaDB error 1193 (Unknown system variable).
		if err := driverConfig.Apply(mysqlDriver.Charset(strings.TrimSpace(raw[0]), "")); err != nil {
			return nil, fmt.Errorf("DB_PARAMS charset is invalid: %w", err)
		}
		delete(values, "charset")
	}
	for name, items := range values {
		if len(items) != 1 {
			return nil, fmt.Errorf("DB_PARAMS value %q must be specified once", name)
		}
		driverConfig.Params[name] = items[0]
	}
	return driverConfig, nil
}

// SQLConnector returns the MariaDB driver connector for Platform-owned SQL lifecycle management.
func (c Config) SQLConnector() (driver.Connector, error) {
	driverConfig, err := c.mysqlDriverConfig()
	if err != nil {
		return nil, err
	}
	return mysqlDriver.NewConnector(driverConfig)
}

// RedisOptions returns connection options for Platform-owned Redis lifecycle management.
func (c Config) RedisOptions() redis.Options {
	return redis.Options{
		Addr:         c.RedisAddr,
		Password:     c.RedisPassword,
		DB:           c.RedisDB,
		DialTimeout:  c.RedisDialTimeout,
		ReadTimeout:  c.RedisReadTimeout,
		WriteTimeout: c.RedisWriteTimeout,
	}
}

func (c Config) RedisKey(key string) string { return c.RedisKeyPrefix + strings.TrimLeft(key, ":") }

// EscapePassword is provided for diagnostics only; do not log DSNs or credentials.
func EscapePassword(s string) string { return url.QueryEscape(s) }
