package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"example.com/stock-analysis-flow/internal/server/bootstrap"
)

func main() {
	// 先安装默认日志，保证 Bootstrap 初始化失败时仍可输出结构化错误。
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Run(ctx); err != nil {
		slog.Error("股票分析服务退出", "service", "stock-analysis-api", "error", err.Error())
		os.Exit(1)
	}
}
