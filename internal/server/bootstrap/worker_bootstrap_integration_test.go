package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

const workerBootstrapHelperEnv = "SERVER_WORKER_BOOTSTRAP_HELPER"

type workerBootstrapOutput struct {
	mu   sync.Mutex
	text strings.Builder
}

func (o *workerBootstrapOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.text.Write(p)
}

func (o *workerBootstrapOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.text.String()
}

// TestWorkerBootstrapHelperProcess 在独立进程中运行真实 Server Worker Bootstrap，
// 由父测试发送 SIGTERM，覆盖 Run 的信号取消与资源释放路径。
func TestWorkerBootstrapHelperProcess(t *testing.T) {
	if os.Getenv(workerBootstrapHelperEnv) != "1" {
		return
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot := filepath.Clean(filepath.Join(workingDir, "..", "..", ".."))
	if err := os.Chdir(repositoryRoot); err != nil {
		t.Fatalf("切换到仓库根目录失败: %v", err)
	}

	ctx, stop := signalNotifyContext()
	defer stop()
	if err := Run(ctx); err != nil {
		t.Fatalf("Worker Bootstrap Run() 失败: %v", err)
	}
}

// signalNotifyContext 单独封装以避免测试辅助进程与父进程共享取消状态。
func signalNotifyContext() (context.Context, func()) {
	return signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
}

func TestWorkerBootstrapMariaDBRedisIntegration(t *testing.T) {
	adminDSN := strings.TrimSpace(os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN"))
	redisAddr := strings.TrimSpace(os.Getenv("PLATFORM_REDIS_ADDR"))
	if adminDSN == "" || redisAddr == "" {
		t.Skip("需要真实 MariaDB DSN 和 Redis 地址；Integration CI 会提供这两个依赖")
	}

	adminConfig, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatalf("解析 MariaDB admin DSN 失败: %v", err)
	}
	if adminConfig.Net != "tcp" {
		t.Fatalf("MariaDB admin DSN network = %q，当前集成测试要求 tcp", adminConfig.Net)
	}
	admin, err := sql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatalf("打开 MariaDB admin 连接失败: %v", err)
	}
	defer admin.Close()

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer setupCancel()
	if err := admin.PingContext(setupCtx); err != nil {
		t.Fatalf("连接 MariaDB admin 失败: %v", err)
	}
	host, port, err := net.SplitHostPort(adminConfig.Addr)
	if err != nil {
		t.Fatalf("解析 MariaDB 地址失败: %v", err)
	}
	database := fmt.Sprintf("worker_bootstrap_%d", time.Now().UTC().UnixNano())
	if _, err := admin.ExecContext(setupCtx, "CREATE DATABASE `"+database+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("创建隔离 Worker 数据库失败: %v", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS `"+database+"`"); err != nil {
			t.Errorf("删除隔离 Worker 数据库失败: %v", err)
		}
	}()

	// 父进程占用 HTTP_ADDR。Worker 角色必须完全跳过 HTTP 配置和监听器装配；
	// 同时提供非法 HTTP_READ_TIMEOUT，防止意外加载 HTTP 配置被静默掩盖。
	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占用 HTTP 测试地址失败: %v", err)
	}
	defer httpListener.Close()

	values := map[string]string{
		workerBootstrapHelperEnv:                "1",
		"SERVER_ROLE":                           "worker",
		"DB_HOST":                               host,
		"DB_PORT":                               port,
		"DB_USER":                               adminConfig.User,
		"DB_PASSWORD":                           adminConfig.Passwd,
		"DB_NAME":                               database,
		"DB_AUTO_MIGRATE":                       "true",
		"DB_PARAMS":                             "charset=utf8mb4&parseTime=true&loc=UTC",
		"REDIS_ADDR":                            redisAddr,
		"REDIS_PASSWORD":                        os.Getenv("REDIS_PASSWORD"),
		"REDIS_DB":                              os.Getenv("REDIS_DB"),
		"MARKET_DATA_BASE_URL":                  "http://127.0.0.1:8091",
		"MARKET_DATA_API_KEY":                   "worker-bootstrap-integration-secret-0123456789",
		"RESEARCHFLOW_MAX_QUEUED_RUNS":          "100",
		"RESEARCHFLOW_MAX_WORKERS":              "8",
		"RESEARCHFLOW_RUN_DEADLINE":             "45m",
		"RESEARCHFLOW_LOCAL_WORKERS":            "2",
		"MIGRATION_ENABLED":                     "true",
		"NOTIFICATION_EVENT_MAX_ENVELOPE_BYTES": "65536",
		"NOTIFICATION_EVENT_MAX_PAYLOAD_BYTES":  "32768",
		"NOTIFICATION_EVENT_MAX_JSON_DEPTH":     "32",
		"HTTP_ADDR":                             httpListener.Addr().String(),
		"HTTP_READ_TIMEOUT":                     "not-a-duration",
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerBootstrapHelperProcess$")
	cmd.Env = workerBootstrapEnvironment(values)
	output := &workerBootstrapOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动 Worker Bootstrap 子进程失败: %v", err)
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	finished := false
	defer func() {
		if !finished {
			_ = cmd.Process.Kill()
			<-waitDone
		}
	}()

	startupDeadline := time.Now().Add(60 * time.Second)
	for {
		if strings.Contains(output.String(), "股票分析 Worker 已启动") {
			break
		}
		select {
		case err := <-waitDone:
			finished = true
			t.Fatalf("Worker 未完成启动，子进程提前退出: %v\n%s", err, output.String())
		default:
		}
		if time.Now().After(startupDeadline) {
			t.Fatalf("Worker 在 60 秒内未完成启动\n%s", output.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("向 Worker 发送 SIGTERM 失败: %v", err)
	}
	select {
	case err := <-waitDone:
		finished = true
		if err != nil {
			t.Fatalf("Worker 收到 SIGTERM 后退出失败: %v\n%s", err, output.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Worker 收到 SIGTERM 后未在 20 秒内完成关闭")
	}
	if !strings.Contains(output.String(), "股票分析 Worker 已启动") {
		t.Fatalf("Worker 启动日志缺失: %s", output.String())
	}
}

func workerBootstrapEnvironment(overrides map[string]string) []string {
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[key]; replaced {
			continue
		}
		switch key {
		case "SERVER_RELAY_TEST_MYSQL_DSN", "PLATFORM_REDIS_ADDR", "PLATFORM_MARIADB_ADDR",
			"PLATFORM_MARIADB_DATABASE", "PLATFORM_MARIADB_USER", "PLATFORM_MARIADB_PASSWORD":
			continue
		}
		env = append(env, entry)
	}
	for key, value := range overrides {
		if value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}
