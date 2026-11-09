package storage

import (
	"context"
	"database/sql"
	"errors"

	mysql "github.com/go-sql-driver/mysql"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

// openNotificationIntegrationDB 让 MariaDB 集成测试复用生产装配使用的 Platform 连接池初始化。
func openNotificationIntegrationDB(ctx context.Context, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, errors.New("notification storage: integration DSN is required")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	connector, err := mysql.NewConnector(config)
	if err != nil {
		return nil, err
	}
	return platformstorage.OpenSQL(ctx, connector, platformstorage.DefaultSQLConfig())
}
