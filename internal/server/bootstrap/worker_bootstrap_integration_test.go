package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
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

	platformfault "example.com/stock-analysis-flow/internal/platform/fault"
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
		var platformErr *platformfault.Fault
		if errors.As(err, &platformErr) && platformErr.Cause != nil {
			t.Fatalf("Server Bootstrap Run() 失败: %v（Fault code：%s；内部原因：%v）", err, platformErr.Code, platformErr.Cause)
		}
		t.Fatalf("Server Bootstrap Run() 失败: %v", err)
	}
}

// signalNotifyContext 单独封装以避免测试辅助进程与父进程共享取消状态。
func signalNotifyContext() (context.Context, func()) {
	return signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
}

type workerBootstrapChild struct {
	cmd      *exec.Cmd
	output   *workerBootstrapOutput
	waitDone chan error
	finished bool
}

func startWorkerBootstrapChild(t *testing.T, values map[string]string) *workerBootstrapChild {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerBootstrapHelperProcess$")
	cmd.Env = workerBootstrapEnvironment(values)
	output := &workerBootstrapOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动 Server Bootstrap 子进程失败: %v", err)
	}
	child := &workerBootstrapChild{
		cmd: cmd, output: output, waitDone: make(chan error, 1),
	}
	go func() { child.waitDone <- cmd.Wait() }()
	return child
}

func (c *workerBootstrapChild) waitReady(t *testing.T) {
	t.Helper()
	startupDeadline := time.Now().Add(60 * time.Second)
	for {
		if strings.Contains(c.output.String(), "股票分析 Worker 已启动") {
			return
		}
		select {
		case err := <-c.waitDone:
			c.finished = true
			t.Fatalf("Server 子进程未完成启动，提前退出: %v\n%s", err, c.output.String())
		default:
		}
		if time.Now().After(startupDeadline) {
			t.Fatalf("Server 子进程在 60 秒内未完成启动\n%s", c.output.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (c *workerBootstrapChild) stop(t *testing.T) {
	t.Helper()
	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("向 Server 子进程发送 SIGTERM 失败: %v", err)
	}
	select {
	case err := <-c.waitDone:
		c.finished = true
		if err != nil {
			t.Fatalf("Server 子进程收到 SIGTERM 后退出失败: %v\n%s", err, c.output.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("Server 子进程收到 SIGTERM 后未在 20 秒内完成关闭\n%s", c.output.String())
	}
}

func (c *workerBootstrapChild) killIfRunning() {
	if c == nil || c.finished {
		return
	}
	_ = c.cmd.Process.Kill()
	<-c.waitDone
	c.finished = true
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
	children := make([]*workerBootstrapChild, 0, 2)
	defer func() {
		for _, child := range children {
			child.killIfRunning()
		}
	}()

	// 两个独立 Server 进程共享同一 MariaDB/Redis 和容量配置。
	// Relay Worker ID 按进程区分，避免部署时误用相同的消费者身份。
	for index := 1; index <= 2; index++ {
		childValues := make(map[string]string, len(values)+1)
		for key, value := range values {
			childValues[key] = value
		}
		childValues["RESEARCH_NOTIFICATION_RELAY_WORKER_ID"] = fmt.Sprintf("integration-worker-%d", index)
		children = append(children, startWorkerBootstrapChild(t, childValues))
	}

	for _, child := range children {
		child.waitReady(t)
	}
	for _, child := range children {
		child.stop(t)
	}
	for index, child := range children {
		if !strings.Contains(child.output.String(), "股票分析 Worker 已启动") {
			t.Fatalf("Server 子进程 %d 启动日志缺失: %s", index+1, child.output.String())
		}
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

func (c *workerBootstrapChild) waitAPIReady(t *testing.T, baseURL, ownerToken string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(60 * time.Second)
	for {
		request, err := http.NewRequest(http.MethodGet, baseURL+"/readyz", nil)
		if err != nil {
			t.Fatalf("构造 API Readiness 请求失败: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+ownerToken)
		response, requestErr := client.Do(request)
		if requestErr == nil {
			status := response.StatusCode
			_ = response.Body.Close()
			if status == http.StatusOK {
				return
			}
		}
		select {
		case childErr := <-c.waitDone:
			c.finished = true
			t.Fatalf("API Server 在 Readiness 通过前退出: %v\n%s", childErr, c.output.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("API Server 在 60 秒内未达到 Readiness\n%s", c.output.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestAPIBootstrapMariaDBRedisIntegration(t *testing.T) {
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
	database := fmt.Sprintf("api_bootstrap_%d", time.Now().UTC().UnixNano())
	if _, err := admin.ExecContext(setupCtx, "CREATE DATABASE `"+database+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("创建隔离 API 数据库失败: %v", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS `"+database+"`"); err != nil {
			t.Errorf("删除隔离 API 数据库失败: %v", err)
		}
	}()

	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("分配 API 测试监听地址失败: %v", err)
	}
	httpAddress := httpListener.Addr().String()
	if err := httpListener.Close(); err != nil {
		t.Fatalf("释放 API 测试监听地址失败: %v", err)
	}

	ownerToken := "api-bootstrap-integration-owner-token-0123456789"
	values := map[string]string{
		workerBootstrapHelperEnv:                "1",
		"SERVER_ROLE":                           "api",
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
		"MARKET_DATA_API_KEY":                   "api-bootstrap-integration-secret-0123456789",
		"MIGRATION_ENABLED":                     "true",
		"PLATFORM_ADMIN_TOKEN":                  ownerToken,
		"HTTP_ADDR":                             httpAddress,
		"HTTP_SHUTDOWN_TIMEOUT":                 "10s",
		"HTTP_READ_TIMEOUT":                     "5s",
		"HTTP_MAX_REQUEST_BYTES":                "1024",
		"NOTIFICATION_EVENT_MAX_ENVELOPE_BYTES": "65536",
		"NOTIFICATION_EVENT_MAX_PAYLOAD_BYTES":  "32768",
		"NOTIFICATION_EVENT_MAX_JSON_DEPTH":     "32",
	}

	child := startWorkerBootstrapChild(t, values)
	defer child.killIfRunning()
	baseURL := "http://" + httpAddress
	child.waitAPIReady(t, baseURL, ownerToken)

	client := &http.Client{Timeout: 2 * time.Second}
	healthRequest, err := http.NewRequest(http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		t.Fatalf("构造 Health 请求失败: %v", err)
	}
	healthResponse, err := client.Do(healthRequest)
	if err != nil {
		t.Fatalf("API Health 请求失败: %v\n%s", err, child.output.String())
	}
	healthStatus := healthResponse.StatusCode
	_ = healthResponse.Body.Close()
	if healthStatus != http.StatusOK {
		t.Fatalf("匿名 /healthz 状态码 = %d，期望 200", healthStatus)
	}

	anonymousReadyRequest, err := http.NewRequest(http.MethodGet, baseURL+"/readyz", nil)
	if err != nil {
		t.Fatalf("构造匿名 Readiness 请求失败: %v", err)
	}
	anonymousReadyResponse, err := client.Do(anonymousReadyRequest)
	if err != nil {
		t.Fatalf("匿名 Readiness 请求失败: %v", err)
	}
	anonymousReadyStatus := anonymousReadyResponse.StatusCode
	_ = anonymousReadyResponse.Body.Close()
	if anonymousReadyStatus != http.StatusUnauthorized {
		t.Fatalf("匿名 /readyz 状态码 = %d，期望 401", anonymousReadyStatus)
	}

	// Root 必须在真实 Data InProcessHandler 前执行 Owner Bearer 认证。
	unauthorizedRequest, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/bars/INVALID/daily", nil)
	if err != nil {
		t.Fatalf("构造未认证 Data 请求失败: %v", err)
	}
	unauthorizedResponse, err := client.Do(unauthorizedRequest)
	if err != nil {
		t.Fatalf("未认证 Data 请求失败: %v", err)
	}
	unauthorizedStatus := unauthorizedResponse.StatusCode
	_ = unauthorizedResponse.Body.Close()
	if unauthorizedStatus != http.StatusUnauthorized {
		t.Fatalf("未认证 Data 路由状态码 = %d，期望 401", unauthorizedStatus)
	}

	// 非法 Symbol 在 Data Handler 的参数校验阶段被拒绝，不触发任何 Provider 调用。
	// 不携带 X-API-Key，证明进程内 Data Handler 复用 Root Owner Bearer。
	dataRequest, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/bars/INVALID/daily", nil)
	if err != nil {
		t.Fatalf("构造 Data 路由请求失败: %v", err)
	}
	dataRequest.Header.Set("Authorization", "Bearer "+ownerToken)
	dataRequest.Header.Set("X-Request-ID", "caller-supplied-request-id")
	dataResponse, err := client.Do(dataRequest)
	if err != nil {
		t.Fatalf("真实 Data InProcessHandler 请求失败: %v", err)
	}
	if dataResponse.StatusCode != http.StatusBadRequest {
		_ = dataResponse.Body.Close()
		t.Fatalf("Data 非法 Symbol 状态码 = %d，期望 400", dataResponse.StatusCode)
	}
	requestID := dataResponse.Header.Get("X-Request-ID")
	if requestID == "" || requestID == "caller-supplied-request-id" {
		_ = dataResponse.Body.Close()
		t.Fatalf("Root 未生成可信 X-Request-ID: %q", requestID)
	}
	var dataError struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(dataResponse.Body).Decode(&dataError); err != nil {
		_ = dataResponse.Body.Close()
		t.Fatalf("解析 Data Validation Fault 失败: %v", err)
	}
	_ = dataResponse.Body.Close()
	if dataError.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("Data Validation Fault code = %q，期望 VALIDATION_ERROR", dataError.Error.Code)
	}

	// Root 请求体限制必须覆盖 Data Probe 路由，并返回可关联的 Request ID。
	oversizedBody := strings.NewReader(strings.Repeat("x", 1025))
	probeRequest, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/data-sources/probe", oversizedBody)
	if err != nil {
		t.Fatalf("构造超限 Probe 请求失败: %v", err)
	}
	probeRequest.Header.Set("Authorization", "Bearer "+ownerToken)
	probeRequest.Header.Set("Content-Type", "application/json")
	probeResponse, err := client.Do(probeRequest)
	if err != nil {
		t.Fatalf("超限 Probe 请求失败: %v", err)
	}
	if probeResponse.StatusCode != http.StatusRequestEntityTooLarge {
		_ = probeResponse.Body.Close()
		t.Fatalf("超限 Probe 状态码 = %d，期望 413", probeResponse.StatusCode)
	}
	var oversizedError publicErrorEnvelope
	if err := json.NewDecoder(probeResponse.Body).Decode(&oversizedError); err != nil {
		_ = probeResponse.Body.Close()
		t.Fatalf("解析 Root 请求体超限 Fault 失败: %v", err)
	}
	probeRequestID := probeResponse.Header.Get("X-Request-ID")
	_ = probeResponse.Body.Close()
	if oversizedError.Error.Code != "server.request_too_large" {
		t.Fatalf("请求体超限 Fault code = %q，期望 server.request_too_large", oversizedError.Error.Code)
	}
	if probeRequestID == "" || oversizedError.Error.RequestID != probeRequestID {
		t.Fatalf("请求体超限 Request ID 不一致: header=%q body=%q", probeRequestID, oversizedError.Error.RequestID)
	}

	child.stop(t)

	// Migration disabled 必须先校验 history，再由 Data Ready 检查物理结构；
	// 缺列时拒绝启动，且不得在 disabled 模式下自动修复数据库。
	dbConfig := *adminConfig
	dbConfig.DBName = database
	applicationDB, err := sql.Open("mysql", dbConfig.FormatDSN())
	if err != nil {
		t.Fatalf("打开隔离 API 数据库失败: %v", err)
	}
	defer applicationDB.Close()
	if _, err := applicationDB.ExecContext(setupCtx, "ALTER TABLE market_data_quotes DROP COLUMN turnover_rate"); err != nil {
		t.Fatalf("模拟 Migration disabled 下的物理 Schema 漂移失败: %v", err)
	}

	disabledValues := make(map[string]string, len(values))
	for key, value := range values {
		disabledValues[key] = value
	}
	disabledValues["MIGRATION_ENABLED"] = "false"
	disabledValues["DB_AUTO_MIGRATE"] = "false"
	disabledChild := startWorkerBootstrapChild(t, disabledValues)
	defer disabledChild.killIfRunning()
	select {
	case childErr := <-disabledChild.waitDone:
		disabledChild.finished = true
		if childErr == nil {
			t.Fatalf("Migration disabled 时物理 Schema 缺列，Server 仍然启动成功：%s", disabledChild.output.String())
		}
	case <-time.After(15 * time.Second):
		disabledChild.killIfRunning()
		t.Fatalf("Migration disabled 时物理 Schema 缺列，Server 未在 15 秒内 fail-closed：%s", disabledChild.output.String())
	}
	startupOutput := disabledChild.output.String()
	if !strings.Contains(startupOutput, "market_data.schema_drift") ||
		!strings.Contains(startupOutput, "market_data_quotes.turnover_rate is missing") {
		t.Fatalf("Migration disabled 启动失败未返回预期 Schema drift 证据：%s", startupOutput)
	}
	var remainingColumns int
	if err := applicationDB.QueryRowContext(setupCtx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'market_data_quotes' AND column_name = 'turnover_rate'`).Scan(&remainingColumns); err != nil {
		t.Fatalf("检查 Migration disabled 是否改写物理 Schema 失败: %v", err)
	}
	if remainingColumns != 0 {
		t.Fatalf("Migration disabled 自动修改了物理 Schema，turnover_rate column count = %d", remainingColumns)
	}
}
