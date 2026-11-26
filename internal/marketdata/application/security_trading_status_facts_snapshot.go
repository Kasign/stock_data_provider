package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/snapshotport"
)

var ErrSecurityTradingStatusFactsSnapshotCreationUnavailable = errors.New("trading-status facts snapshot creator unavailable")

// SecurityTradingStatusPITReader 只读本地 append-only 版本表，绝不回落到实时 Provider。
type SecurityTradingStatusPITReader interface {
	SecurityTradingStatusAsOf(ctx context.Context, symbol, tradeDate string, asOf time.Time) (*model.SecurityTradingStatusSnapshot, error)
}

type SecurityTradingStatusFactsSnapshotReferences struct {
	SnapshotRef      string
	AuthorizationRef string
}

// SecurityTradingStatusFactsSnapshotCreator 是交易状态事实快照的唯一生产入口。
// 它不返回 Payload；下游必须通过授权读取入口获取事实。
type SecurityTradingStatusFactsSnapshotCreator struct {
	reader     SecurityTradingStatusPITReader
	repository snapshotport.SecurityTradingStatusFactsSnapshotRepository
	owner      *DataAuthorizationOwner
}

func NewSecurityTradingStatusFactsSnapshotCreator(
	reader SecurityTradingStatusPITReader,
	repository snapshotport.SecurityTradingStatusFactsSnapshotRepository,
	owner *DataAuthorizationOwner,
) (*SecurityTradingStatusFactsSnapshotCreator, error) {
	if reader == nil || repository == nil || owner == nil {
		return nil, ErrSecurityTradingStatusFactsSnapshotCreationUnavailable
	}
	return &SecurityTradingStatusFactsSnapshotCreator{reader: reader, repository: repository, owner: owner}, nil
}

// Create 读取一次 PIT 交易状态，构建并持久化不可变事实快照，最后签发绑定的交易事实数据类授权。
// 授权签发失败时允许留下不可变快照，但绝不返回可消费引用。
func (c *SecurityTradingStatusFactsSnapshotCreator) Create(
	ctx context.Context,
	symbol, tradeDate string,
	systemAsOf time.Time,
	purpose string,
) (SecurityTradingStatusFactsSnapshotReferences, error) {
	if c == nil || c.reader == nil || c.repository == nil || c.owner == nil || ctx == nil {
		return SecurityTradingStatusFactsSnapshotReferences{}, ErrSecurityTradingStatusFactsSnapshotCreationUnavailable
	}
	if symbol == "" || symbol != strings.TrimSpace(symbol) {
		return SecurityTradingStatusFactsSnapshotReferences{}, errors.New("trading-status facts snapshot symbol is required and must be canonical")
	}
	if !validTradingFactsDate(tradeDate) {
		return SecurityTradingStatusFactsSnapshotReferences{}, errors.New("trading-status facts snapshot requires a valid trade_date")
	}
	if systemAsOf.IsZero() || systemAsOf.Location() != time.UTC {
		return SecurityTradingStatusFactsSnapshotReferences{}, errors.New("trading-status facts snapshot requires a UTC system_as_of")
	}
	_, callerRef, _, err := trustedDataPrincipal(ctx)
	if err != nil {
		return SecurityTradingStatusFactsSnapshotReferences{}, err
	}
	if strings.TrimSpace(callerRef) == "" {
		return SecurityTradingStatusFactsSnapshotReferences{}, ErrDataAuthorizationDenied
	}

	record, err := c.reader.SecurityTradingStatusAsOf(ctx, symbol, tradeDate, systemAsOf)
	if err != nil {
		return SecurityTradingStatusFactsSnapshotReferences{}, fmt.Errorf("read PIT trading status for Snapshot: %w", err)
	}
	payload, err := buildTradingStatusFactsPayload(record, symbol, tradeDate)
	if err != nil {
		return SecurityTradingStatusFactsSnapshotReferences{}, err
	}
	snapshot, err := app.BuildSecurityTradingStatusFactsSnapshot(
		payload, tradingStatusFactsSources(record), callerRef, purpose, systemAsOf,
	)
	if err != nil {
		return SecurityTradingStatusFactsSnapshotReferences{}, fmt.Errorf("build trading-status facts Snapshot: %w", err)
	}
	if err := c.repository.SaveSecurityTradingStatusFactsSnapshot(ctx, snapshot); err != nil {
		return SecurityTradingStatusFactsSnapshotReferences{}, fmt.Errorf("persist trading-status facts Snapshot: %w", err)
	}
	authorizationRef, err := c.owner.IssueFor(
		ctx, snapshot.Manifest.SnapshotRef, purpose, authorization.SecurityStatusFactsPolicyRef,
	)
	if err != nil {
		return SecurityTradingStatusFactsSnapshotReferences{}, fmt.Errorf("Snapshot persisted but authorization was not issued: %w", err)
	}
	return SecurityTradingStatusFactsSnapshotReferences{
		SnapshotRef: snapshot.Manifest.SnapshotRef, AuthorizationRef: authorizationRef,
	}, nil
}

// buildTradingStatusFactsPayload 把 Data 记录的 PIT 交易状态原样映射为五项事实。
//
// 三条硬约束：
//  1. 只有 Data 真正记录过的值才是 available；枚举里的 unknown 一律表达为 unavailable，
//     不能因为"字段有默认值"就当成已核验。
//  2. 涨跌停状态与可成交量不属于本 Payload：Provider 从未提供，写入等于把推导伪装成事实。
//  3. 源记录自身为 invalid/unknown 时直接拒绝；源记录为 degraded 而五项事实恰好齐全时也拒绝，
//     否则会把 Data 的降级结论升级成 valid，等于替 Data 改判。
func buildTradingStatusFactsPayload(
	record *model.SecurityTradingStatusSnapshot,
	symbol, tradeDate string,
) (app.SecurityTradingStatusFactsPayload, error) {
	if record == nil {
		return app.SecurityTradingStatusFactsPayload{}, errors.New("trading-status facts source record is required")
	}
	if record.Symbol != symbol || record.TradeDate != tradeDate {
		return app.SecurityTradingStatusFactsPayload{}, errors.New("trading-status facts source record identity does not match the request")
	}
	switch record.QualityStatus {
	case model.QualityValid, model.QualityDegraded:
	default:
		return app.SecurityTradingStatusFactsPayload{}, fmt.Errorf(
			"trading-status facts source record quality %q is not usable evidence", record.QualityStatus)
	}

	facts := []app.SecurityTradingStatusFact{
		tradingStatusFact(
			app.TradingFactRiskWarning,
			string(record.RiskWarningStatus),
			record.RiskWarningStatus != "" && record.RiskWarningStatus != model.RiskWarningUnknown,
			"risk_warning_status_unknown",
		),
		tradingStatusFact(
			app.TradingFactSuspension,
			string(record.SuspensionStatus),
			record.SuspensionStatus != "" && record.SuspensionStatus != model.SuspensionStatusUnknown,
			"suspension_status_unknown",
		),
		tradingStatusFact(
			app.TradingFactDelisting,
			string(record.DelistingStatus),
			record.DelistingStatus != "" && record.DelistingStatus != model.DelistingStatusUnknown,
			"delisting_status_unknown",
		),
		tradingStatusFact(
			app.TradingFactListingDate,
			record.ListingDate,
			record.ListingStatusKnown && validTradingFactsDate(record.ListingDate),
			"listing_status_unknown",
		),
		tradingStatusFact(
			app.TradingFactTradingDay,
			tradingDayValue(record.IsTradingDay),
			record.TradingDayKnown,
			"trading_day_unknown",
		),
	}

	quality := app.QualityValid
	for _, fact := range facts {
		if fact.Availability == app.TradingFactUnavailable {
			quality = app.QualityDegraded
			break
		}
	}
	if record.QualityStatus == model.QualityDegraded && quality == app.QualityValid {
		return app.SecurityTradingStatusFactsPayload{}, errors.New(
			"degraded trading-status record has no representable degradation in the facts payload")
	}
	if strings.TrimSpace(record.Source) == "" {
		return app.SecurityTradingStatusFactsPayload{}, errors.New("trading-status facts source record has no source")
	}
	return app.SecurityTradingStatusFactsPayload{
		SchemaVersion: app.SecurityTradingStatusFactsPayloadSchemaVersion,
		PayloadKind:   app.SecurityTradingStatusFactsPayloadKind,
		Symbol:        symbol,
		TradeDate:     tradeDate,
		QualityStatus: quality,
		Facts:         facts,
	}, nil
}

func tradingStatusFact(key app.TradingFactKey, value string, available bool, reason string) app.SecurityTradingStatusFact {
	if available {
		return app.SecurityTradingStatusFact{
			Key: string(key), Availability: app.TradingFactAvailable, Value: value,
		}
	}
	return app.SecurityTradingStatusFact{
		Key: string(key), Availability: app.TradingFactUnavailable, UnavailableReason: reason,
	}
}

func tradingDayValue(isTradingDay bool) string {
	if isTradingDay {
		return "true"
	}
	return "false"
}

// tradingStatusFactsSources 只使用记录里真实存在的来源信息。
// SourceVersion 为空时保持 nil，不能填接口版本或空串冒称数据版本。
func tradingStatusFactsSources(record *model.SecurityTradingStatusSnapshot) []app.DailyBarsCloseSeriesSource {
	source := app.DailyBarsCloseSeriesSource{SourceRef: record.Source}
	if strings.TrimSpace(record.SourceVersion) != "" {
		version := record.SourceVersion
		source.SourceVersion = &version
	}
	return []app.DailyBarsCloseSeriesSource{source}
}

func validTradingFactsDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}
