package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
)

func TestNewsRetentionComponentMariaDBIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 复用 Server 现有真实 MariaDB 隔离库和 News Migration，不另建连接池或复制 SQL。
	db := openNewsResearchMariaDB(t, ctx)
	repository, err := newsstorage.NewMySQLRepository(db)
	if err != nil {
		t.Fatalf("创建 News MySQL Repository 失败: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	expired := make([]newsdomain.Observation, 3)
	for index := range expired {
		expired[index] = newsdomain.Observation{
			ID:             fmt.Sprintf("server-news-retention-%d-%d", now.UnixNano(), index),
			IdempotencyKey: fmt.Sprintf("server-news-retention-key-%d-%d", now.UnixNano(), index),
			SourceID:       "server-news-retention-integration",
			Candidate: newsdomain.Candidate{
				SourceItemID: fmt.Sprintf("server-retention-source-item-%d", index),
				Title:        "到期后必须清除的候选载荷",
			},
			ObservedAt:     now.Add(-2 * time.Hour),
			RetentionUntil: now.Add(-time.Minute),
			IdentityStatus: newsdomain.ObservationUnidentified,
		}
		if err := repository.WithinIdentityTransaction(ctx, func(tx newsapp.IdentityTransaction) error {
			_, appendErr := tx.AppendObservation(ctx, expired[index])
			return appendErr
		}); err != nil {
			t.Fatalf("写入到期 Observation %s 失败: %v", expired[index].ID, err)
		}
	}

	// 未到期记录作为边界样本，必须保留原始载荷。
	active := newsdomain.Observation{
		ID:             fmt.Sprintf("server-news-retention-active-%d", now.UnixNano()),
		IdempotencyKey: fmt.Sprintf("server-news-retention-active-key-%d", now.UnixNano()),
		SourceID:       "server-news-retention-integration",
		Candidate: newsdomain.Candidate{
			SourceItemID: "server-retention-active-item",
			Title:        "未到期载荷必须保留",
		},
		ObservedAt:     now.Add(-time.Minute),
		RetentionUntil: now.Add(time.Hour),
		IdentityStatus: newsdomain.ObservationUnidentified,
	}
	if err := repository.WithinIdentityTransaction(ctx, func(tx newsapp.IdentityTransaction) error {
		_, appendErr := tx.AppendObservation(ctx, active)
		return appendErr
	}); err != nil {
		t.Fatalf("写入未到期 Observation 失败: %v", err)
	}

	component, err := newNewsRetentionComponent(
		repository,
		serverconfig.NewsRetention{Interval: time.Minute, BatchSize: 2},
		nil,
	)
	if err != nil {
		t.Fatalf("构造 Server News Retention Component 失败: %v", err)
	}
	component.clock = newsRetentionTestClock{now: now}

	// Root Component 每次执行必须遵守模块 Cleaner 的批次上限；连续执行后应清空积压。
	component.runOnce(ctx)
	assertNewsRetentionMariaDBState(t, ctx, db, expired, active, 2)
	component.runOnce(ctx)
	assertNewsRetentionMariaDBState(t, ctx, db, expired, active, 3)

	// 再执行一次不得重复清理，也不得删除 Tombstone 或未到期记录。
	component.runOnce(ctx)
	assertNewsRetentionMariaDBState(t, ctx, db, expired, active, 3)
}

func assertNewsRetentionMariaDBState(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	expired []newsdomain.Observation,
	active newsdomain.Observation,
	wantPurged int,
) {
	t.Helper()

	purged := 0
	for _, observation := range expired {
		var candidateJSON []byte
		var purgedAt sql.NullTime
		if err := db.QueryRowContext(ctx,
			"SELECT candidate_json, payload_purged_at FROM news_observations WHERE observation_id = ?",
			observation.ID,
		).Scan(&candidateJSON, &purgedAt); err != nil {
			t.Fatalf("读取到期 Observation %s 失败: %v", observation.ID, err)
		}
		if purgedAt.Valid {
			purged++
			var candidate newsdomain.Candidate
			if err := json.Unmarshal(candidateJSON, &candidate); err != nil {
				t.Fatalf("解析已清理候选载荷失败: %v", err)
			}
			if candidate.Title != "" {
				t.Fatalf("到期载荷未清除: observation=%s title=%q", observation.ID, candidate.Title)
			}
		} else {
			var candidate newsdomain.Candidate
			if err := json.Unmarshal(candidateJSON, &candidate); err != nil {
				t.Fatalf("解析待清理候选载荷失败: %v", err)
			}
			if candidate.Title == "" {
				t.Fatalf("未清理记录的候选载荷意外丢失: observation=%s", observation.ID)
			}
		}
	}
	if purged != wantPurged {
		t.Fatalf("已清理 Observation 数量 = %d，期望 %d", purged, wantPurged)
	}

	var activePayload []byte
	var activePurgedAt sql.NullTime
	if err := db.QueryRowContext(ctx,
		"SELECT candidate_json, payload_purged_at FROM news_observations WHERE observation_id = ?",
		active.ID,
	).Scan(&activePayload, &activePurgedAt); err != nil {
		t.Fatalf("读取未到期 Observation 失败: %v", err)
	}
	var activeCandidate newsdomain.Candidate
	if err := json.Unmarshal(activePayload, &activeCandidate); err != nil {
		t.Fatalf("解析未到期候选载荷失败: %v", err)
	}
	if activePurgedAt.Valid || activeCandidate.Title != active.Candidate.Title {
		t.Fatalf("未到期记录被错误清理: purged_at=%+v title=%q", activePurgedAt, activeCandidate.Title)
	}
}
