package mysqlredis

import (
	"testing"

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
	if !driverConfig.ParseTime || driverConfig.Loc.String() != "UTC" || driverConfig.Params["charset"] != "utf8mb4" {
		t.Fatalf("DB_PARAMS not applied correctly: parseTime=%t location=%q params=%v", driverConfig.ParseTime, driverConfig.Loc, driverConfig.Params)
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
