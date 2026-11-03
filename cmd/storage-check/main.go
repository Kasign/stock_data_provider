package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
	"example.com/stock-analysis-flow/internal/platform/config"
)

func main() {
	if err := config.LoadDotEnvIfExists(".env"); err != nil {
		fail(err)
	}
	cfg, err := mysqlredis.LoadConfigFromEnv()
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := mysqlredis.Open(ctx, cfg)
	if err != nil {
		fail(err)
	}
	defer store.Close()
	if cfg.DBAutoMigrate {
		fmt.Println("MariaDB/Redis 连接成功；DB_AUTO_MIGRATE=true，已执行数据库迁移（幂等）。")
	} else {
		fmt.Println("MariaDB/Redis 连接成功；DB_AUTO_MIGRATE=false，未执行数据库迁移。")
	}
	if err := store.SelfTest(ctx); err != nil {
		fail(err)
	}
	fmt.Println("MariaDB 表结构、事务写入/读取/回滚，以及 Redis SET/GET/DEL 往返测试均通过。")
}
func fail(err error) { fmt.Fprintln(os.Stderr, "storage-check:", err); os.Exit(1) }
