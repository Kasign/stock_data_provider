// Command limit-pool-collect 在 Run 之前把当日涨跌停事实固化成不可变快照。
//
// 它存在的理由：quant.risk 的 system_as_of 是 Run 创建时刻，而事实采集必然晚于
// Run 创建，因此「Stage 内现场采集」在 PIT 上不合法（见 DEC-PLAT-002-076）。
// 采集必须在 Run 之前完成，读取侧按 as-of 取「不晚于评估时点」的最新快照。
//
// 权限边界：本命令只创建快照，**不签发任何授权**。授权仍由 Risk Stage 在读取时
// 签发，因此本工具不需要 Owner 级凭据。
//
// 注意：它使用 Provider 的默认 HTTP 传输，而不是 Root Server 注入的出站策略执行器；
// 目标地址是固定的公开行情端点，不接受用户输入 URL。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	"example.com/stock-analysis-flow/internal/marketdata/cache"
	marketdataconfig "example.com/stock-analysis-flow/internal/marketdata/config"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
	"example.com/stock-analysis-flow/internal/marketdata/service"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

// collectOptions 是本命令的全部输入；它只接受具名参数，不读未知环境变量。
type collectOptions struct {
	symbol     string
	tradeDate  string
	purpose    string
	systemAsOf time.Time
	timeout    time.Duration
}

// collectorPrincipalSubject 是采集器写入快照 caller_ref 的身份。
// 它只影响快照归属与指纹，不授予访问权限。
const collectorPrincipalSubject = "limit-pool-collect"

func main() {
	if err := platformconfig.LoadDotEnvIfExists(".env"); err != nil {
		fail(err)
	}
	options, err := parseCollectOptions(os.Args[1:], time.Now().UTC())
	if err != nil {
		fail(err)
	}
	if err := run(options); err != nil {
		fail(err)
	}
}

// parseCollectOptions 解析并校验参数。now 由调用方注入，便于测试确定性行为。
func parseCollectOptions(args []string, now time.Time) (collectOptions, error) {
	flags := flag.NewFlagSet("limit-pool-collect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	symbol := flags.String("symbol", "", "A 股证券代码，例如 600519.SH（必填）")
	tradeDate := flags.String("trade-date", "", "交易日 YYYY-MM-DD（必填，必须是已收盘的交易日）")
	purpose := flags.String("purpose", "risk_review", "事实用途，例如 risk_review")
	systemAsOf := flags.String("system-as-of", "", "评估时点 RFC3339（可选，默认当前 UTC 时刻）")
	timeout := flags.Duration("timeout", 2*time.Minute, "整体超时时间")
	if err := flags.Parse(args); err != nil {
		return collectOptions{}, fmt.Errorf("解析参数失败: %w", err)
	}
	if flags.NArg() > 0 {
		return collectOptions{}, fmt.Errorf("不支持位置参数: %s", strings.Join(flags.Args(), " "))
	}

	options := collectOptions{
		symbol:    strings.TrimSpace(*symbol),
		tradeDate: strings.TrimSpace(*tradeDate),
		purpose:   strings.TrimSpace(*purpose),
		timeout:   *timeout,
	}
	if options.symbol == "" {
		return collectOptions{}, errors.New("-symbol 必填")
	}
	if options.tradeDate == "" {
		return collectOptions{}, errors.New("-trade-date 必填")
	}
	parsedDate, err := time.Parse("2006-01-02", options.tradeDate)
	if err != nil || parsedDate.Format("2006-01-02") != options.tradeDate {
		return collectOptions{}, fmt.Errorf("-trade-date 必须是 YYYY-MM-DD: %q", options.tradeDate)
	}
	if options.purpose == "" {
		return collectOptions{}, errors.New("-purpose 不能为空")
	}
	if options.timeout <= 0 {
		return collectOptions{}, errors.New("-timeout 必须为正数")
	}

	if raw := strings.TrimSpace(*systemAsOf); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, raw)
		if parseErr != nil {
			return collectOptions{}, fmt.Errorf("-system-as-of 必须是 RFC3339: %w", parseErr)
		}
		options.systemAsOf = parsed.UTC()
	}
	// 未指定时保持零值：采集器会以实际观测时刻作为快照时点。
	// 若在这里取「现在」，观测必然晚于它，PIT 会正确拒绝。
	_ = now
	return options, nil
}

func run(options collectOptions) error {
	cfg, err := marketdataconfig.LoadForCollector()
	if err != nil {
		return fmt.Errorf("加载 Data 配置失败: %w", err)
	}
	if strings.TrimSpace(cfg.MySQLDSN) == "" {
		return errors.New("未配置 MariaDB：请设置 MARKET_DATA_DB_HOST/USER/NAME 或 MARKET_DATA_MYSQL_DSN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), options.timeout)
	defer cancel()

	store, err := repository.OpenMySQL(ctx, cfg.MySQLDSN)
	if err != nil {
		return fmt.Errorf("连接 MariaDB 失败: %w", err)
	}
	defer store.Close()
	// 采集器不做迁移：Schema 由 Server 启动流程统一管理，这里只确认可用。
	if err := store.ValidateSchema(ctx); err != nil {
		return fmt.Errorf("Data Schema 不可用（需先由 Server 完成迁移）: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	timeout := cfg.ProviderTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	client := provider.NewEastmoney(timeout, "", "")
	reader := service.NewWithCapabilities(
		[]provider.NamedProvider{client}, nil, nil, cache.NewDisabled(), store,
		time.Minute, time.Minute, 120, 2000, service.ResilienceConfig{}, logger,
	)
	reader.SetLimitPoolProviders([]provider.LimitPoolProvider{client})

	collector, err := marketdataapplication.NewLimitPoolFactsSnapshotCollector(reader, store)
	if err != nil {
		return err
	}
	principalCtx, err := platformsecurity.WithPrincipal(ctx, platformsecurity.Principal{
		Kind:       platformsecurity.IdentityService,
		AuthMethod: platformsecurity.AuthMethodServiceBearer,
		Subject:    collectorPrincipalSubject,
		Scopes:     []string{platformsecurity.ScopeDataRead},
	})
	if err != nil {
		return fmt.Errorf("构造采集身份失败: %w", err)
	}

	snapshotRef, err := collector.Persist(principalCtx, options.symbol, options.tradeDate, options.systemAsOf, options.purpose)
	if err != nil {
		return fmt.Errorf("采集涨跌停事实失败: %w", err)
	}
	// 读回一次，确认快照真的落库且可解析；采集器不签发授权，因此这里走非授权读取。
	snapshot, err := store.GetLimitPoolFactsSnapshot(ctx, snapshotRef)
	if err != nil {
		return fmt.Errorf("快照已写入但读回失败: %w", err)
	}
	if err := snapshot.Verify(); err != nil {
		return fmt.Errorf("快照读回后指纹校验失败: %w", err)
	}
	fmt.Printf("limit-pool-collect: 完成 symbol=%s trade_date=%s purpose=%s system_as_of=%s\n",
		options.symbol, options.tradeDate, options.purpose, snapshot.Manifest.SystemAsOf)
	fmt.Printf("limit-pool-collect: status=%s up=%d down=%d quality=%s\n",
		snapshot.Payload.LimitStatus, snapshot.Payload.UpPoolTotal, snapshot.Payload.DownPoolTotal, snapshot.Payload.QualityStatus)
	fmt.Printf("limit-pool-collect: snapshot_ref=%s\n", snapshotRef)
	fmt.Println("limit-pool-collect: 未签发授权（授权由 Risk Stage 读取时签发）")
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "limit-pool-collect:", err)
	os.Exit(1)
}
