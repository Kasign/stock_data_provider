package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sort"

	marketdataapp "example.com/stock-analysis-flow/internal/marketdata/app"
	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
)

// quantDataSnapshotPort is the narrow Data-owned Snapshot boundary consumed by Quant.
// The concrete Server dependency is marketdata/bootstrap.Module.
type quantDataSnapshotPort interface {
	CreateDailyBarsCloseSeriesSnapshot(context.Context, marketdataapp.DailyBarsQuery, string) (marketdataapplication.DailyBarsCloseSeriesSnapshotReferences, error)
	ReadAuthorizedDailyBarsCloseSeriesSnapshot(context.Context, string, string, string) (marketdataapp.DailyBarsCloseSeriesSnapshot, error)
}

type quantDataSnapshotAdapter struct {
	data quantDataSnapshotPort
}

func newQuantDataSnapshotAdapter(data quantDataSnapshotPort) (*quantDataSnapshotAdapter, error) {
	if data == nil {
		return nil, errors.New("Quant Data Snapshot adapter requires the Data Module")
	}
	return &quantDataSnapshotAdapter{data: data}, nil
}

func (adapter *quantDataSnapshotAdapter) CreateDailyBarsCloseSeriesSnapshot(
	ctx context.Context,
	request quantstage.SnapshotCreateRequest,
) (quantstage.SnapshotReferences, error) {
	if adapter == nil || adapter.data == nil || ctx == nil {
		return quantstage.SnapshotReferences{}, errors.New("Quant Data Snapshot creator is unavailable")
	}
	cutoff := request.SystemAsOf.UTC()
	refs, err := adapter.data.CreateDailyBarsCloseSeriesSnapshot(ctx, marketdataapp.DailyBarsQuery{
		Symbol: request.Symbol, StartDate: request.StartDate, EndDate: request.EndDate,
		Limit: request.Limit, AdjustmentBasis: marketdataapp.AdjustmentNone,
		SystemObservedAtCutoff: &cutoff,
		AllowedProviderCodes:   append([]string(nil), request.AllowedProviderCodes...),
	}, quantstage.SMA20PurposeCode)
	if err != nil {
		return quantstage.SnapshotReferences{}, fmt.Errorf("Data CreateDailyBarsCloseSeriesSnapshot: %w", err)
	}
	return quantstage.SnapshotReferences{
		SnapshotRef: refs.SnapshotRef, AuthorizationRef: refs.AuthorizationRef,
	}, nil
}

func (adapter *quantDataSnapshotAdapter) ReadAuthorizedCloseSeriesSnapshot(
	ctx context.Context,
	snapshotRef, authorizationRef, purposeCode string,
) (quantapplication.CloseSeriesSnapshot, error) {
	if adapter == nil || adapter.data == nil || ctx == nil {
		return quantapplication.CloseSeriesSnapshot{}, errors.New("Quant Data Snapshot reader is unavailable")
	}
	snapshot, err := adapter.data.ReadAuthorizedDailyBarsCloseSeriesSnapshot(
		ctx, snapshotRef, authorizationRef, purposeCode,
	)
	if err != nil {
		return quantapplication.CloseSeriesSnapshot{}, fmt.Errorf("Data ReadAuthorizedDailyBarsCloseSeriesSnapshot: %w", err)
	}
	if err := snapshot.Verify(); err != nil {
		return quantapplication.CloseSeriesSnapshot{}, fmt.Errorf("Data returned an invalid close-series Snapshot: %w", err)
	}
	manifest := snapshot.Manifest
	payload := snapshot.Payload
	if manifest.SnapshotRef != snapshotRef || manifest.PurposeCode != purposeCode {
		return quantapplication.CloseSeriesSnapshot{}, errors.New("Data authorized Snapshot identity does not match the requested reference and purpose")
	}
	sources := make([]quantapplication.CloseSeriesSource, len(manifest.Sources))
	for index, source := range manifest.Sources {
		var sourceVersion *string
		if source.SourceVersion != nil {
			value := *source.SourceVersion
			sourceVersion = &value
		}
		sources[index] = quantapplication.CloseSeriesSource{
			SourceRef: source.SourceRef, SourceVersion: sourceVersion,
			ProviderInterfaceVersion: source.ProviderInterfaceVersion,
		}
	}
	bars := make([]quantapplication.CloseSeriesBar, len(payload.Bars))
	for index, bar := range payload.Bars {
		bars[index] = quantapplication.CloseSeriesBar{
			TradeDate: bar.TradeDate, Close: bar.Close, QualityStatus: string(bar.QualityStatus),
		}
	}
	return quantapplication.CloseSeriesSnapshot{
		SnapshotRef:           manifest.SnapshotRef,
		ManifestSchemaVersion: manifest.SchemaVersion,
		PayloadSchemaVersion:  payload.SchemaVersion,
		PayloadKind:           payload.PayloadKind,
		CallerRef:             manifest.CallerRef,
		PurposeCode:           manifest.PurposeCode,
		Sources:               sources,
		Symbol:                manifest.Symbol,
		StartDate:             manifest.StartDate,
		EndDate:               manifest.EndDate,
		AdjustmentBasis:       string(manifest.AdjustmentBasis),
		SystemAsOf:            manifest.SystemAsOf,
		Interval:              payload.Interval,
		TradingCalendarRef:    payload.TradingCalendarRef,
		QualityStatus:         string(payload.QualityStatus),
		Coverage: quantapplication.CloseSeriesCoverageEvidence{
			CalendarQualityStatus: string(payload.CoverageEvidence.CalendarQualityStatus),
			ExpectedTradingDays:   payload.CoverageEvidence.ExpectedTradingDays,
			MinimumRequiredBars:   payload.CoverageEvidence.MinimumRequiredBars,
			CoverageSufficient:    payload.CoverageEvidence.CoverageSufficient,
		},
		Bars: bars,
	}, nil
}

var _ quantstage.DailyBarsCloseSeriesSnapshotCreator = (*quantDataSnapshotAdapter)(nil)
var _ quantapplication.CloseSeriesSnapshotReader = (*quantDataSnapshotAdapter)(nil)

// quantDailyBarsProviderCodes 返回 Quant Stage 可用的 DailyBars Provider 清单。
//
// Quant Stage 只在受支持的 Provider 实际装配时注册；未装配时返回空切片，由调用方
// 决定跳过组合而不是降级到其他 Provider。SMA 与 Factor 共用同一份判定，避免两处漂移。
func quantDailyBarsProviderCodes(
	dataModule *marketdatabootstrap.Module,
	providerCode string,
) []string {
	if dataModule == nil {
		return nil
	}
	providers := append([]string(nil), dataModule.ProviderSummary().DailyBars...)
	sort.Strings(providers)
	if containsQuantProviderCode(providers, providerCode) {
		return []string{providerCode}
	}
	return nil
}

// mergeQuantProviderCodes 合并多个 Quant Stage 的 Provider 清单，
// 保持去重与升序，满足 ResourcePolicy 对固定 inventory 的不可变约束。
func mergeQuantProviderCodes(lists ...[]string) []string {
	seen := make(map[string]struct{})
	merged := make([]string, 0, len(lists))
	for _, list := range lists {
		for _, code := range list {
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			merged = append(merged, code)
		}
	}
	sort.Strings(merged)
	return merged
}
