package application

import (
	"context"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/snapshotport"
)

var ErrDailyBarsCloseSeriesSnapshotCreationUnavailable = errors.New("daily-bars close-series snapshot creator unavailable")

type DailyBarsCloseSeriesSnapshotReferences struct {
	SnapshotRef      string
	AuthorizationRef string
}

// DailyBarsCloseSeriesSnapshotCreator 是 Data 内部唯一的首批 SMA Snapshot 生产入口。
// 它不返回 Payload；下游必须通过授权读取入口获取数据。
type DailyBarsCloseSeriesSnapshotCreator struct {
	reader     app.DailyBarsV2Reader
	repository snapshotport.DailyBarsCloseSeriesSnapshotRepository
	owner      *DataAuthorizationOwner
}

func NewDailyBarsCloseSeriesSnapshotCreator(
	reader app.DailyBarsV2Reader,
	repository snapshotport.DailyBarsCloseSeriesSnapshotRepository,
	owner *DataAuthorizationOwner,
) (*DailyBarsCloseSeriesSnapshotCreator, error) {
	if reader == nil || repository == nil || owner == nil {
		return nil, ErrDailyBarsCloseSeriesSnapshotCreationUnavailable
	}
	return &DailyBarsCloseSeriesSnapshotCreator{reader: reader, repository: repository, owner: owner}, nil
}

// Create 读取一次 PIT V2 日线，构建并持久化不可变 Snapshot，最后签发绑定该 Snapshot 的授权。
// 授权签发失败时允许留下不可变 Snapshot，但绝不返回可消费引用。
func (c *DailyBarsCloseSeriesSnapshotCreator) Create(
	ctx context.Context,
	query app.DailyBarsQuery,
	purpose string,
) (DailyBarsCloseSeriesSnapshotReferences, error) {
	if c == nil || c.reader == nil || c.repository == nil || c.owner == nil || ctx == nil {
		return DailyBarsCloseSeriesSnapshotReferences{}, ErrDailyBarsCloseSeriesSnapshotCreationUnavailable
	}
	_, callerRef, _, err := trustedDataPrincipal(ctx)
	if err != nil {
		return DailyBarsCloseSeriesSnapshotReferences{}, err
	}
	read, err := c.reader.GetDailyBarsV2(ctx, query)
	if err != nil {
		return DailyBarsCloseSeriesSnapshotReferences{}, fmt.Errorf("read PIT daily bars for Snapshot: %w", err)
	}
	input, err := app.BuildDailyBarsCloseSeriesInput(query, read)
	if err != nil {
		return DailyBarsCloseSeriesSnapshotReferences{}, fmt.Errorf("build daily-bars close-series input: %w", err)
	}
	snapshot, err := app.BuildDailyBarsCloseSeriesSnapshot(input, callerRef, purpose)
	if err != nil {
		return DailyBarsCloseSeriesSnapshotReferences{}, fmt.Errorf("build daily-bars close-series Snapshot: %w", err)
	}
	if err := c.repository.SaveDailyBarsCloseSeriesSnapshot(ctx, snapshot); err != nil {
		return DailyBarsCloseSeriesSnapshotReferences{}, fmt.Errorf("persist daily-bars close-series Snapshot: %w", err)
	}
	authorizationRef, err := c.owner.Issue(ctx, snapshot.Manifest.SnapshotRef, purpose)
	if err != nil {
		return DailyBarsCloseSeriesSnapshotReferences{}, fmt.Errorf("Snapshot persisted but authorization was not issued: %w", err)
	}
	return DailyBarsCloseSeriesSnapshotReferences{
		SnapshotRef: snapshot.Manifest.SnapshotRef, AuthorizationRef: authorizationRef,
	}, nil
}
