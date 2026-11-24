package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
)

func TestDailyBarsCloseSeriesSnapshotStorageDisabledFailsClosed(t *testing.T) {
	snapshot := validRepositoryDailyBarsCloseSeriesSnapshot(t)
	if err := (*Store)(nil).SaveDailyBarsCloseSeriesSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("SaveDailyBarsCloseSeriesSnapshot() succeeded without storage")
	}
	if _, err := (*Store)(nil).GetDailyBarsCloseSeriesSnapshot(context.Background(), snapshot.Manifest.SnapshotRef); err == nil {
		t.Fatal("GetDailyBarsCloseSeriesSnapshot() succeeded without storage")
	}
	if err := (&Store{}).SaveDailyBarsCloseSeriesSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("SaveDailyBarsCloseSeriesSnapshot() succeeded with an unconfigured Store")
	}
}

func TestDailyBarsCloseSeriesSnapshotRefRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{"", "data_snapshot:sha256:xyz", "data_snapshot:sha256:" + strings.Repeat("A", 64)} {
		if _, err := parseDailyBarsCloseSeriesSnapshotRef(value); err == nil {
			t.Fatalf("malformed SnapshotRef was accepted: %q", value)
		}
	}
}

func TestMariaDBDailyBarsCloseSeriesSnapshotIsImmutableAndVerifiable(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		dsn = os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	}
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}
	if err := store.ValidateCompatibilitySchema(ctx); err != nil {
		t.Fatalf("validate physical Data schema: %v", err)
	}

	snapshot := validRepositoryDailyBarsCloseSeriesSnapshot(t)
	if err := store.SaveDailyBarsCloseSeriesSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("save immutable close-series snapshot: %v", err)
	}
	if err := store.SaveDailyBarsCloseSeriesSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("idempotent duplicate save failed: %v", err)
	}
	loaded, err := store.GetDailyBarsCloseSeriesSnapshot(ctx, snapshot.Manifest.SnapshotRef)
	if err != nil {
		t.Fatalf("read immutable close-series snapshot: %v", err)
	}
	if err := loaded.Verify(); err != nil {
		t.Fatalf("loaded snapshot failed Verify(): %v", err)
	}
	if loaded.Manifest.SnapshotRef != snapshot.Manifest.SnapshotRef ||
		loaded.Manifest.ManifestFingerprint != snapshot.Manifest.ManifestFingerprint ||
		loaded.PayloadFingerprint != snapshot.PayloadFingerprint ||
		loaded.Payload.Bars[0].Close != snapshot.Payload.Bars[0].Close {
		t.Fatalf("loaded Snapshot differs from the saved content: %+v", loaded)
	}

	if _, err := store.GetDailyBarsCloseSeriesSnapshot(ctx, "data_snapshot:sha256:"+strings.Repeat("0", 64)); !errors.Is(err, ErrDailyBarsCloseSeriesSnapshotNotFound) {
		t.Fatalf("unknown SnapshotRef must return not-found, got %v", err)
	}
	if _, err := store.GetDailyBarsCloseSeriesSnapshot(ctx, "data_snapshot:sha256:invalid"); err == nil {
		t.Fatal("malformed SnapshotRef must be rejected")
	}

	// Concurrent duplicate writes must converge on the same immutable record.
	const writers = 4
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errs <- store.SaveDailyBarsCloseSeriesSnapshot(ctx, snapshot)
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent idempotent Snapshot save failed: %v", err)
		}
	}

	manifestJSON, payloadJSON, err := canonicalDailyBarsCloseSeriesSnapshotJSON(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE market_data_daily_bars_close_series_snapshots SET payload_json='{}' WHERE snapshot_ref=?", snapshot.Manifest.SnapshotRef); err != nil {
		t.Fatalf("tamper with stored Payload: %v", err)
	}
	if _, err := store.GetDailyBarsCloseSeriesSnapshot(ctx, snapshot.Manifest.SnapshotRef); err == nil {
		t.Fatal("tampered Payload must fail content verification")
	}
	if err := store.SaveDailyBarsCloseSeriesSnapshot(ctx, snapshot); err == nil {
		t.Fatal("Save must not overwrite a conflicting/tampered immutable record")
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE market_data_daily_bars_close_series_snapshots SET manifest_json=?, payload_json=? WHERE snapshot_ref=?", string(manifestJSON), string(payloadJSON), snapshot.Manifest.SnapshotRef); err != nil {
		t.Fatalf("restore stored Snapshot after integrity test: %v", err)
	}
}

func validRepositoryDailyBarsCloseSeriesSnapshot(t *testing.T) app.DailyBarsCloseSeriesSnapshot {
	t.Helper()
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	observed := cutoff.Add(-time.Minute)
	fractionFour, fractionThree := 4, 3
	query := app.DailyBarsQuery{
		Symbol: "600036.SH", StartDate: "2026-11-17", EndDate: "2026-11-18",
		Limit: 2, AdjustmentBasis: app.AdjustmentNone, SystemObservedAtCutoff: &cutoff,
	}
	snapshot := app.DailyBarsV2Snapshot{
		Symbol: "600036.SH", Interval: "1d", AdjustmentBasis: app.AdjustmentNone,
		QualityStatus: app.QualityValid, SystemObservedAtCutoff: &cutoff,
		TradingCalendarRef: repositoryStringPointer("data_calendar:sha256:" + strings.Repeat("a", 64)),
		CoverageEvidence: app.DailyBarCoverageEvidence{
			CalendarQualityStatus: app.QualityValid, ExpectedTradingDays: 2, MinimumRequiredBars: 2,
			CoverageBasis: "exchange_calendar", CoverageSufficient: true,
		},
		Bars: []app.DailyBar{
			repositoryCloseSeriesBar("2026-11-17", "43.49", "43.4900", &fractionFour, cutoff, observed),
			repositoryCloseSeriesBar("2026-11-18", "43.12", "43.120", &fractionThree, cutoff, observed),
		},
	}
	input, err := app.BuildDailyBarsCloseSeriesInput(query, snapshot)
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesInput() error = %v", err)
	}
	result, err := app.BuildDailyBarsCloseSeriesSnapshot(input, "principal:repository-test", "quant_analysis")
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesSnapshot() error = %v", err)
	}
	return result
}

func repositoryCloseSeriesBar(tradeDate, closeValue, sourceToken string, fractionDigits *int, cutoff, observed time.Time) app.DailyBar {
	return app.DailyBar{
		Symbol: "600036.SH", TradeDate: tradeDate, Period: "1d", AdjustmentBasis: app.AdjustmentNone,
		Source: "eastmoney", SourceVersion: "eastmoney-stock-api-v1",
		FetchedAt: cutoff.Add(-2 * time.Minute), NormalizedAt: cutoff.Add(-time.Minute),
		SystemObservedAt: &observed, NormalizationVersion: "4", QualityStatus: app.QualityValid,
		ExactValues: &app.DailyBarExactValues{
			Close: closeValue,
			CloseEvidence: &app.DailyBarDecimalEvidence{
				EvidenceKind: "observed_token", SourceToken: sourceToken, SourceFractionDigits: fractionDigits,
			},
		},
	}
}

func repositoryStringPointer(value string) *string { return &value }

func TestDataAuthorizationPolicyStorageDisabledFailsClosed(t *testing.T) {
	if err := (*Store)(nil).PublishDataAuthorizationPolicyVersion(context.Background(), "", authorization.PolicyVersion{}, app.DataAuthorizationPolicyPublicationAudit{}); err == nil {
		t.Fatal("publishing policy without storage must fail")
	}
	if _, err := (*Store)(nil).ResolveCurrentDataAuthorizationPolicy(context.Background()); err == nil {
		t.Fatal("resolving current policy without storage must fail")
	}
}

func TestMariaDBDataAuthorizationPolicyVersionsAreImmutableAndCurrent(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		dsn = os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	}
	if dsn == "" {
		t.Skip("MariaDB integration DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}
	if err := store.ValidateCompatibilitySchema(ctx); err != nil {
		t.Fatalf("validate Data schema: %v", err)
	}

	if _, err := store.ResolveCurrentDataAuthorizationPolicy(ctx); !errors.Is(err, app.ErrDataAuthorizationPolicyNotPublished) {
		t.Fatalf("missing current policy must be reported as not-published, got %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	version := fmt.Sprintf("integration-%d", now.UnixNano())
	policy := authorization.DataAccessPolicy{
		SchemaVersion:   authorization.DataAccessPolicySchemaVersion,
		Status:          authorization.PolicyApproved,
		AllowedSubjects: []string{"principal:service:market-data-it"},
		AllowedPurposes: []string{"research_analysis"},
		RequiredScopes:  []string{"data:read"},
		EffectiveAt:     now.Add(-time.Minute),
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	value := authorization.PolicyVersion{
		PolicyType:    authorization.DataAccessPolicyType,
		PolicyRef:     authorization.DailyBarsV2PolicyRef,
		PolicyVersion: version,
		Payload:       payload,
		PayloadHash:   hex.EncodeToString(sum[:]),
		CreatedAt:     now,
	}
	if err := store.PublishDataAuthorizationPolicyVersion(ctx, "", value, policyPublicationAudit(now, version)); err != nil {
		t.Fatalf("publish policy version: %v", err)
	}
	current, err := store.ResolveCurrentDataAuthorizationPolicy(ctx)
	if err != nil {
		t.Fatalf("resolve current policy: %v", err)
	}
	if current.Version.PolicyVersion != version || current.Policy.Status != authorization.PolicyApproved {
		t.Fatalf("current policy mismatch: %+v", current)
	}

	// current pointer 指向不存在的版本属于存储完整性故障，不能误报为未发布。
	missingVersion := version + "-missing-target"
	if _, err := store.db.ExecContext(ctx, `UPDATE market_data_current_authorization_policy_versions
SET policy_version=? WHERE policy_type=? AND policy_ref=?`,
		missingVersion, authorization.DataAccessPolicyType, authorization.DailyBarsV2PolicyRef); err != nil {
		t.Fatalf("point current policy at missing version for integrity test: %v", err)
	}
	if _, err := store.ResolveCurrentDataAuthorizationPolicy(ctx); !errors.Is(err, ErrDataAuthorizationPolicyUnavailable) {
		t.Fatalf("dangling current policy pointer must be unavailable, got %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE market_data_current_authorization_policy_versions
SET policy_version=? WHERE policy_type=? AND policy_ref=?`,
		version, authorization.DataAccessPolicyType, authorization.DailyBarsV2PolicyRef); err != nil {
		t.Fatalf("restore current policy pointer after integrity test: %v", err)
	}

	stale := value
	stale.PolicyVersion = version + "-stale"
	if err := store.PublishDataAuthorizationPolicyVersion(ctx, "stale-current-version", stale, policyPublicationAudit(now.Add(time.Second), stale.PolicyVersion)); !errors.Is(err, ErrDataAuthorizationPolicyConflict) {
		t.Fatalf("stale expected current version must conflict, got %v", err)
	}
	current, err = store.ResolveCurrentDataAuthorizationPolicy(ctx)
	if err != nil || current.Version.PolicyVersion != version {
		t.Fatalf("stale policy publication changed current pointer: current=%+v err=%v", current, err)
	}
	var staleVersionCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM market_data_authorization_policy_versions
WHERE policy_type=? AND policy_ref=? AND policy_version=?`,
		authorization.DataAccessPolicyType, authorization.DailyBarsV2PolicyRef, stale.PolicyVersion).Scan(&staleVersionCount); err != nil || staleVersionCount != 0 {
		t.Fatalf("stale publication must roll back its immutable version row: count=%d err=%v", staleVersionCount, err)
	}

	changed := value
	changedPolicy := policy
	changedPolicy.Status = authorization.PolicyRevoked
	changed.Payload, err = json.Marshal(changedPolicy)
	if err != nil {
		t.Fatal(err)
	}
	changedSum := sha256.Sum256(changed.Payload)
	changed.PayloadHash = hex.EncodeToString(changedSum[:])
	if err := store.PublishDataAuthorizationPolicyVersion(ctx, version, changed, policyPublicationAudit(now.Add(2*time.Second), changed.PolicyVersion)); !errors.Is(err, ErrDataAuthorizationPolicyConflict) {
		t.Fatalf("same policy version with different content must conflict, got %v", err)
	}
	current, err = store.ResolveCurrentDataAuthorizationPolicy(ctx)
	if err != nil || current.Version.PolicyVersion != version || current.Policy.Status != authorization.PolicyApproved {
		t.Fatalf("conflicting immutable write changed current policy: current=%+v err=%v", current, err)
	}

	revoked := policy
	revoked.Status = authorization.PolicyRevoked
	revokedPayload, err := json.Marshal(revoked)
	if err != nil {
		t.Fatal(err)
	}
	revokedSum := sha256.Sum256(revokedPayload)
	revokedVersion := value
	revokedVersion.PolicyVersion = version + "-revoked"
	revokedVersion.Payload = revokedPayload
	revokedVersion.PayloadHash = hex.EncodeToString(revokedSum[:])
	revokedVersion.CreatedAt = now.Add(time.Second)
	if err := store.PublishDataAuthorizationPolicyVersion(ctx, version, revokedVersion, policyPublicationAudit(now.Add(3*time.Second), revokedVersion.PolicyVersion)); err != nil {
		t.Fatalf("publish revoked policy: %v", err)
	}
	current, err = store.ResolveCurrentDataAuthorizationPolicy(ctx)
	if err != nil || current.Policy.Status != authorization.PolicyRevoked || current.Version.PolicyVersion != revokedVersion.PolicyVersion {
		t.Fatalf("revoked policy did not become current: current=%+v err=%v", current, err)
	}
	if err := store.PublishDataAuthorizationPolicyVersion(ctx, revokedVersion.PolicyVersion, value, policyPublicationAudit(now.Add(4*time.Second), value.PolicyVersion)); !errors.Is(err, ErrDataAuthorizationPolicyConflict) {
		t.Fatalf("historical policy version must not be reactivated, got %v", err)
	}
	current, err = store.ResolveCurrentDataAuthorizationPolicy(ctx)
	if err != nil || current.Version.PolicyVersion != revokedVersion.PolicyVersion || current.Policy.Status != authorization.PolicyRevoked {
		t.Fatalf("historical policy reactivation changed current pointer: current=%+v err=%v", current, err)
	}
	var publicationAuditCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM market_data_authorization_policy_publications
WHERE policy_type=? AND policy_ref=?`, authorization.DataAccessPolicyType, authorization.DailyBarsV2PolicyRef).Scan(&publicationAuditCount); err != nil {
		t.Fatalf("count policy publication audit: %v", err)
	}
	if publicationAuditCount != 2 {
		t.Fatalf("successful publications should have exactly two audit rows, got %d", publicationAuditCount)
	}

}

func policyPublicationAudit(at time.Time, suffix string) app.DataAuthorizationPolicyPublicationAudit {
	return app.DataAuthorizationPolicyPublicationAudit{
		ActorRef: "principal:owner", RequestID: "data-policy-it-" + suffix, OccurredAt: at.UTC(),
	}
}
