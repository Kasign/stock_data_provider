package mysqlredis

import (
	"strings"
	"testing"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

func TestMySQLDriverConfigPreservesSpecialCharactersInCredentials(t *testing.T) {
	const (
		username = "service@user"
		password = "P@ss:word/with?query#fragment&dollar$percent%"
	)
	cfg := Config{
		DBHost: "Mariadb", DBPort: 3306,
		DBUser: username, DBPassword: password, DBName: "stock_analysis",
		DBParams: "charset=utf8mb4&parseTime=true&loc=UTC",
	}

	driverConfig, err := cfg.mysqlDriverConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mysqlDriver.NewConnector(driverConfig); err != nil {
		t.Fatalf("driver-native connector config is invalid: %v", err)
	}
	if driverConfig.User != username || driverConfig.Passwd != password {
		t.Fatalf("credentials were changed by config construction: user=%q passwordPreserved=%t", driverConfig.User, driverConfig.Passwd == password)
	}
	if driverConfig.Net != "tcp" || driverConfig.Addr != "Mariadb:3306" || driverConfig.DBName != "stock_analysis" {
		t.Fatalf("unexpected database target: network=%q address=%q database=%q", driverConfig.Net, driverConfig.Addr, driverConfig.DBName)
	}
	if !driverConfig.ParseTime || driverConfig.Loc.String() != "UTC" {
		t.Fatalf("DB_PARAMS parseTime/loc not applied correctly: parseTime=%t location=%q", driverConfig.ParseTime, driverConfig.Loc)
	}
	if _, ok := driverConfig.Params["charset"]; ok {
		t.Fatal("charset must be configured as a driver option, not a MariaDB session variable")
	}
	if !strings.Contains(driverConfig.FormatDSN(), "charset=utf8mb4") {
		t.Fatalf("charset option was not preserved by driver config: %s", driverConfig.FormatDSN())
	}
	if !driverConfig.AllowNativePasswords || !driverConfig.CheckConnLiveness {
		t.Fatal("driver-native config should preserve the MySQL driver's secure/default connection behavior")
	}
}

func TestMySQLDriverConfigRejectsMalformedParams(t *testing.T) {
	cfg := Config{
		DBHost: "Mariadb", DBPort: 3306, DBUser: "service",
		DBName: "stock_analysis", DBParams: "charset=%zz&parseTime=true",
	}
	if _, err := cfg.mysqlDriverConfig(); err == nil {
		t.Fatal("malformed DB_PARAMS must be rejected rather than silently ignored")
	}
}

func TestMySQLDriverConfigRejectsInvalidParseTime(t *testing.T) {
	cfg := Config{
		DBHost: "Mariadb", DBPort: 3306, DBUser: "service",
		DBName: "stock_analysis", DBParams: "charset=utf8mb4&parseTime=maybe",
	}
	if _, err := cfg.mysqlDriverConfig(); err == nil {
		t.Fatal("invalid DB_PARAMS parseTime must be rejected")
	}
}

func TestMySQLDriverConfigRejectsInvalidLocation(t *testing.T) {
	cfg := Config{
		DBHost: "Mariadb", DBPort: 3306, DBUser: "service",
		DBName: "stock_analysis", DBParams: "parseTime=true&loc=NotARealTimeZone",
	}
	if _, err := cfg.mysqlDriverConfig(); err == nil {
		t.Fatal("invalid DB_PARAMS loc must be rejected")
	}
}

func TestLoadConfigUsesExplicitSource(t *testing.T) {
	values := map[string]string{
		"DB_HOST":         "mariadb.internal",
		"DB_USER":         "service-user",
		"DB_PASSWORD":     "secret-value",
		"DB_NAME":         "research",
		"DB_PORT":         "3307",
		"REDIS_ADDR":      "redis.internal:6380",
		"REDIS_PASSWORD":  "redis-secret",
		"DB_AUTO_MIGRATE": "true",
	}
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.DBHost != "mariadb.internal" || cfg.DBPort != 3307 ||
		cfg.DBPassword != "secret-value" || cfg.RedisAddr != "redis.internal:6380" ||
		cfg.RedisPassword != "redis-secret" || !cfg.DBAutoMigrate {
		t.Fatalf("explicit source was not applied correctly: %+v", cfg)
	}
}

func TestLoadConfigRejectsInvalidExplicitSource(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "DB_PORT" {
			return "not-a-port", true
		}
		return "", false
	})
	if _, err := LoadConfig(env); err == nil {
		t.Fatal("LoadConfig() error = nil, want invalid DB_PORT error")
	}
}
