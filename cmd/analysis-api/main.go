package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/stock-analysis-flow/internal/server/bootstrap"
)

func main() {
	// 先安装默认日志，保证 Bootstrap 初始化失败时仍可输出结构化错误。
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if len(os.Args) > 1 {
		if len(os.Args) == 2 && os.Args[1] == "owner-init" {
			if err := runOwnerInit(); err != nil {
				slog.Error("Owner 一次性初始化失败", "error", err.Error())
				os.Exit(1)
			}
			slog.Info("Owner Credential 初始化成功；请立即删除或卸载一次性 Secret")
			return
		}
		slog.Error("不支持的命令参数")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Run(ctx); err != nil {
		slog.Error("股票分析服务退出", "service", "stock-analysis-api", "error", err.Error())
		os.Exit(1)
	}
}

func runOwnerInit() error {
	secretPath := os.Getenv("OWNER_SESSION_INITIAL_SECRET_FILE")
	if secretPath == "" {
		return errors.New("OWNER_SESSION_INITIAL_SECRET_FILE must point to a one-time secret file")
	}
	info, err := os.Lstat(secretPath)
	if err != nil {
		return fmt.Errorf("读取一次性 Secret 文件状态失败: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("一次性 Secret 必须是普通文件，不能是符号链接")
	}
	if info.Mode().Perm()&0077 != 0 {
		return errors.New("一次性 Secret 文件权限必须限制为仅文件所有者可读写")
	}
	if info.Size() < 1 || info.Size() > 1024 {
		return errors.New("一次性 Secret 文件大小必须为 1～1024 字节")
	}
	secret, err := os.ReadFile(secretPath)
	if err != nil {
		return fmt.Errorf("读取一次性 Secret 失败: %w", err)
	}
	defer clear(secret)
	if len(secret) < 1 || len(secret) > 1024 {
		return errors.New("一次性 Secret 文件大小必须为 1～1024 字节")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return bootstrap.InitializeOwner(ctx, secret)
}
