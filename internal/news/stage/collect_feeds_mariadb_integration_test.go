package stage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	provideradapter "example.com/stock-analysis-flow/internal/news/adapters/provider"
	sourceadapter "example.com/stock-analysis-flow/internal/news/adapters/source"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"

	_ "github.com/go-sql-driver/mysql"
)

type feedStageMariaDBTransport struct {
	body string
}

func (t feedStageMariaDBTransport) Do(_ context.Context, _ string, _ string, request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Host != "feed.example" {
		return nil, errors.New("unexpected Feed Stage integration request")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/rss+xml; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(t.body)),
		Request:    request,
	}, nil
}

func TestNewsFeedCollectionAdapterMariaDBIntegration(t *testing.T) {
	dsn := os.Getenv("NEWS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NEWS_MARIADB_TEST_DSN to run News Feed Stage MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("MariaDB ping: %v", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, newsstorage.MigrationNamespace, newsstorage.Migrations()); err != nil {
		t.Fatalf("apply News migrations: %v", err)
	}
	if err := newsstorage.ValidateSchema(ctx, db); err != nil {
		t.Fatalf("validate News schema: %v", err)
	}
	repository, err := newsstorage.NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	sourceID := fmt.Sprintf("feed-stage-%d", time.Now().UnixNano())
	endpointID := "rss-primary"
	now := time.Now().UTC()
	policy := domain.SourcePolicy{
		Status:               domain.AuthorizationApproved,
		AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
		AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:  []domain.ContentKind{domain.ContentAbstract},
		AllowedStorageModes:  []domain.StorageMode{domain.StorageTransient, domain.StoragePersistentExcerpt},
		MaxRetention:         24 * time.Hour,
		EffectiveAt:          now.Add(-time.Minute),
		EvidenceRef:          "operator-authorization:" + sourceID,
	}
	policyPayload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := newsapp.NewPublishNewsPolicyVersion(repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.ExecuteIfCurrent(ctx, domain.NewsPolicyTypeSource, sourceID, "source-policy-v1", "", policyPayload); err != nil {
		t.Fatalf("publish initial Source policy: %v", err)
	}

	registration := newsapp.SourceEndpointRegistration{
		SourceID: sourceID, EndpointID: endpointID, Enabled: true,
		URL: "https://feed.example/rss", PublisherName: "Integration Publisher",
		PublisherDomain: "publisher.example", AccessMethod: domain.AccessRSS,
		SourcePolicy: policy, SourcePolicyVersionID: "source-policy-v1",
		URLPolicy: domain.SourceURLPolicy{
			AllowedSchemes: []string{"https"},
			AllowedHosts:   []string{"feed.example", "news.example"},
			AllowedPorts:   []uint16{443},
		},
		MaxResponseBytes: 1 << 20,
	}
	catalog, err := sourceadapter.NewStaticSourceCatalog([]newsapp.SourceEndpointRegistration{registration})
	if err != nil {
		t.Fatalf("create StaticSourceCatalog: %v", err)
	}
	feedBody := `<?xml version="1.0"?>
		<rss version="2.0"><channel><title>Integration Feed</title>
		<item><guid isPermaLink="false">native-article-1</guid>
		<title>真实 Feed 入库验证</title>
		<link>https://news.example/article/1</link>
		<description>仅授权摘要应被持久化</description>
		<pubDate>Wed, 25 Nov 2026 10:00:00 GMT</pubDate></item>
		</channel></rss>`
	collector, err := newsapp.NewCollectSourceWithPolicyResolver(
		catalog, feedStageMariaDBTransport{body: feedBody}, repository,
	)
	if err != nil {
		t.Fatalf("create CollectSource: %v", err)
	}
	var sequence atomic.Uint64
	ingester, err := newsapp.NewIngestCollectedSource(repository, func() string {
		return fmt.Sprintf("feed-stage-id-%d-%d", now.UnixNano(), sequence.Add(1))
	}, catalog)
	if err != nil {
		t.Fatalf("create IngestCollectedSource: %v", err)
	}
	adapter, err := NewFeedCollectionAdapter([]newsapp.SourceEndpointRegistration{registration}, collector, ingester)
	if err != nil {
		t.Fatalf("create FeedCollectionAdapter: %v", err)
	}
	resourceRef := SourceEndpointSnapshotRef(registration)
	result, err := adapter.Execute(ctx, "run-feed-stage", CollectFeedsStageCode, []string{resourceRef})
	if err != nil {
		t.Fatalf("execute formal Feed collection Stage: %v", err)
	}
	if result.OutputType != CollectFeedsOutputType || result.SchemaVersion != CollectFeedsSchemaVersion ||
		result.Retryable || len(result.ReasonCodes) != 0 {
		t.Fatalf("Feed Stage result contract = %+v", result)
	}
	if err := ValidateFeedCollectionPayload(result.Payload); err != nil {
		t.Fatalf("validate Feed Stage output: %v", err)
	}
	if err := ValidateFeedCollectionQualitySummary(result.QualitySummary); err != nil {
		t.Fatalf("validate Feed Stage quality summary: %v", err)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "success" || output.SourceCount != 1 || output.CollectedEntryCount != 1 ||
		output.IngestedCount != 1 || len(output.Sources) != 1 || len(output.Sources[0].NewsIDs) != 1 {
		t.Fatalf("unexpected Feed Stage output: %+v", output)
	}
	if strings.Contains(string(result.Payload), "https://") || strings.Contains(string(result.Payload), "仅授权摘要") {
		t.Fatalf("Feed StageOutput leaked URL or article content: %s", result.Payload)
	}

	newsID := output.Sources[0].NewsIDs[0]
	var itemCount, versionCount, observationCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM news_items WHERE news_id = ? AND canonical_url = ?",
		newsID, "https://news.example/article/1").Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM news_item_versions WHERE news_id = ?", newsID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM news_observations WHERE news_id = ? AND source_id = ?", newsID, sourceID).Scan(&observationCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 1 || versionCount != 1 || observationCount != 1 {
		t.Fatalf("Feed Stage persisted rows: items=%d versions=%d observations=%d", itemCount, versionCount, observationCount)
	}

	var currentContentJSON, versionContentJSON []byte
	if err := db.QueryRowContext(ctx, "SELECT content_json FROM news_items WHERE news_id = ?", newsID).Scan(&currentContentJSON); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT content_json FROM news_item_versions WHERE news_id = ? ORDER BY version_no DESC LIMIT 1", newsID).Scan(&versionContentJSON); err != nil {
		t.Fatal(err)
	}
	for label, raw := range map[string][]byte{"current item": currentContentJSON, "latest version": versionContentJSON} {
		var content domain.ContentLayers
		if err := json.Unmarshal(raw, &content); err != nil {
			t.Fatalf("decode %s content: %v", label, err)
		}
		if content.SourceSummary != "仅授权摘要应被持久化" || content.Body != nil || len(content.Attachments) != 0 {
			t.Fatalf("%s content violated abstract-only Source Policy: %+v", label, content)
		}
	}
}

// officialAPIMariaDBTransport returns one immutable JSON snapshot for the
// B-group production-chain integration test. It never reaches the network.
type officialAPIMariaDBTransport struct {
	body string
}

func (t officialAPIMariaDBTransport) Do(_ context.Context, _ string, _ string, request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Host != "www.cninfo.com.cn" {
		return nil, errors.New("unexpected B-group official API request")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(t.body)),
		Request:    request,
	}, nil
}

// diagnosingAvailabilityRecorder keeps the production recorder unchanged while
// exposing the exact rejection reason in integration-test logs.
type diagnosingAvailabilityRecorder struct {
	inner *newsapp.RecordCollectedAvailability
	err   error
}

func (r *diagnosingAvailabilityRecorder) Execute(
	ctx context.Context,
	request newsapp.RecordCollectedAvailabilityRequest,
) ([]domain.AvailabilityEvidence, error) {
	evidence, err := r.inner.Execute(ctx, request)
	if err != nil {
		detail := "no qualified/ingest entry"
		if len(request.Qualified) > 0 && len(request.Ingest) > 0 {
			qualified := request.Qualified[0]
			bound := request.Ingest[0]
			detail = fmt.Sprintf(
				"observation(id=%q news=%q source=%q endpoint=%q sourcePolicy=%q retention=%v purged=%v observed=%v) "+
					"version(id=%q news=%q observation=%q observed=%v) field=%q parser=%q raw=%q binding=%+v",
				bound.Observation.ID, bound.Observation.NewsID, bound.Observation.SourceID,
				bound.Observation.EndpointID, bound.Observation.SourcePolicyVersionID,
				bound.Observation.RetentionUntil, bound.Observation.PayloadPurgedAt, bound.Observation.ObservedAt,
				bound.Version.ID, bound.Version.NewsID, bound.Version.ObservationID, bound.Version.ObservedAt,
				qualified.Qualified.FieldPath, qualified.Qualified.ParserVersion,
				qualified.Qualified.Candidate.RawValue, qualified.Binding,
			)
		}
		r.err = fmt.Errorf("%s: %w", detail, err)
	}
	return evidence, err
}

// TestNewsAvailabilityProductionChainMariaDBIntegration is the B-group
// database gate for G1-G5: a real Feed execution drives Collect → Qualify →
// Ingest → Record → Review through the production use cases, and the resulting
// Evidence/Decision must become visible to the strict PIT read path. The
// qualification is test-scoped (its own ref/rule version and a body hash
// computed from the in-memory snapshot); it does not claim production source
// admission.
func TestNewsAvailabilityProductionChainMariaDBIntegration(t *testing.T) {
	dsn := os.Getenv("NEWS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NEWS_MARIADB_TEST_DSN to run News availability production-chain integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("MariaDB ping: %v", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, newsstorage.MigrationNamespace, newsstorage.Migrations()); err != nil {
		t.Fatalf("apply News migrations: %v", err)
	}
	if err := newsstorage.ValidateSchema(ctx, db); err != nil {
		t.Fatalf("validate News schema: %v", err)
	}
	repository, err := newsstorage.NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	effectiveAt := now.Add(-time.Hour)
	sourceID := fmt.Sprintf("cninfo-b-group-%d", now.UnixNano())
	endpointID := "cninfo-his-announcement"
	providerCode := "cninfo"
	fieldPath := "announcements.announcementTime"
	sourcePolicyVersion := "cninfo-source-policy-b-v1"
	endpointPolicyVersion := "cninfo-endpoint-policy-b-v1"
	providerPolicyRef := "cninfo-provider-output-policy-b"
	providerPolicyVersion := "cninfo-provider-output-policy-b-v1"
	reviewRuleVersion := "cninfo-disclosure-review-b-v1"
	parserVersion := "cninfo-disclosure-date-conservative-v1"
	qualificationRef := "availability-qualification-b-group-test-v1"

	body := `{"announcements":[{"announcementId":"1225649440","announcementTitle":"B组真库验证公告","announcementTime":1793376000000,"adjunctUrl":"https://www.cninfo.com.cn/new/disclosure/detail?announcementId=1225649440"}]}`
	sum := sha256.Sum256([]byte(body))
	bodyHash := hex.EncodeToString(sum[:])

	sourcePolicy := domain.SourcePolicy{
		Status:                domain.AuthorizationApproved,
		AllowedAccessMethods:  []domain.AccessMethod{domain.AccessOfficialAPI},
		AllowedPurposes:       []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:   []domain.ContentKind{domain.ContentAbstract, domain.ContentMetadata},
		AllowedStorageModes:   []domain.StorageMode{domain.StorageTransient, domain.StoragePersistentExcerpt, domain.StoragePersistentMetadata},
		AllowedMetadataFields: []string{fieldPath},
		MaxRetention:          24 * time.Hour,
		EffectiveAt:           effectiveAt,
		EvidenceRef:           "personal-use:" + sourceID,
	}
	endpointPolicy := domain.EndpointPolicy{
		Status:                domain.AuthorizationApproved,
		AllowedAccessMethods:  []domain.AccessMethod{domain.AccessOfficialAPI},
		AllowedPurposes:       []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:   []domain.ContentKind{domain.ContentAbstract, domain.ContentMetadata},
		AllowedStorageModes:   []domain.StorageMode{domain.StorageTransient, domain.StoragePersistentExcerpt, domain.StoragePersistentMetadata},
		AllowedMetadataFields: []string{fieldPath},
		MaxRetention:          func() *time.Duration { d := 24 * time.Hour; return &d }(),
		EffectiveAt:           effectiveAt,
		EvidenceRef:           "personal-use:" + sourceID + ":endpoint",
	}
	providerPolicy := domain.ProviderOutputPolicy{
		Status:                  domain.AuthorizationApproved,
		AllowedPurposes:         []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:     []domain.ContentKind{domain.ContentAbstract, domain.ContentMetadata},
		AllowRequestProcessing:  true,
		AllowedStorageModes:     []domain.StorageMode{domain.StoragePersistentExcerpt, domain.StoragePersistentMetadata},
		AllowedMetadataFields:   []string{fieldPath},
		MaxRequestRetention:     24 * time.Hour,
		MaxPersistenceRetention: 24 * time.Hour,
		EffectiveAt:             effectiveAt,
		EvidenceRef:             "personal-use:" + sourceID + ":provider",
	}

	publisher, err := newsapp.NewPublishNewsPolicyVersion(repository)
	if err != nil {
		t.Fatal(err)
	}
	for _, publication := range []struct {
		policyType, ref, version string
		payload                  any
	}{
		{domain.NewsPolicyTypeSource, sourceID, sourcePolicyVersion, sourcePolicy},
		{domain.NewsPolicyTypeEndpoint, endpointID, endpointPolicyVersion, endpointPolicy},
		{domain.NewsPolicyTypeProviderOutput, providerPolicyRef, providerPolicyVersion, providerPolicy},
	} {
		raw, err := json.Marshal(publication.payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := publisher.ExecuteIfCurrent(ctx, publication.policyType, publication.ref, publication.version, "", raw); err != nil {
			t.Fatalf("publish %s/%s: %v", publication.policyType, publication.ref, err)
		}
	}

	registration := newsapp.SourceEndpointRegistration{
		SourceID: sourceID, EndpointID: endpointID, Enabled: true,
		URL:           "https://www.cninfo.com.cn/new/hisAnnouncement/query",
		PublisherName: "巨潮资讯网", PublisherDomain: "cninfo.com.cn",
		AccessMethod: domain.AccessOfficialAPI,
		SourcePolicy: sourcePolicy, SourcePolicyVersionID: sourcePolicyVersion,
		EndpointPolicy: &endpointPolicy, EndpointPolicyVersionID: endpointPolicyVersion,
		ProviderCode:                  providerCode,
		ProviderOutputPolicyRef:       providerPolicyRef,
		ProviderOutputPolicyVersionID: providerPolicyVersion,
		URLPolicy: domain.SourceURLPolicy{
			AllowedSchemes: []string{"https"},
			AllowedHosts:   []string{"www.cninfo.com.cn"},
			AllowedPorts:   []uint16{443},
		},
		MaxResponseBytes: 1 << 20,
		RequestParams: map[string]string{
			"pageNum": "1", "stock": "600036,gssh0600036", "seDate": "2026-10-01~2026-11-27",
		},
	}
	sourceCatalog, err := sourceadapter.NewStaticSourceCatalog([]newsapp.SourceEndpointRegistration{registration})
	if err != nil {
		t.Fatal(err)
	}
	providerCatalog, err := provideradapter.NewStaticOutputPolicyCatalog([]newsapp.ProviderOutputPolicyBinding{{
		ProviderCode: providerCode, PolicyRef: providerPolicyRef,
		PolicyVersion: providerPolicyVersion, Policy: providerPolicy,
	}})
	if err != nil {
		t.Fatal(err)
	}
	qualification := newsapp.AvailabilitySourceQualification{
		QualificationRef: qualificationRef,
		SourceID:         sourceID, EndpointID: endpointID, ProviderCode: providerCode,
		SourcePolicyVersionID: sourcePolicyVersion, EndpointPolicyVersionID: endpointPolicyVersion,
		ProviderOutputPolicyRef: providerPolicyRef, ProviderOutputPolicyVersionID: providerPolicyVersion,
		FieldPath: fieldPath, ParserVersion: parserVersion, ReviewRuleVersion: reviewRuleVersion,
		DeclaredTimezoneOrOffset: "+08:00",
		AllowedPrecisions:        []domain.AvailabilityTimePrecision{domain.AvailabilityTimePrecisionDisclosureBoundary},
		OfficialSemanticsRef:     "test-only:disclosure-date-semantics",
		DateBoundarySemanticsRef: "DEC-PLAT-002-073:disclosure-date-end-of-day-v1",
		EndpointAuthorizationRef: "personal-use:" + sourceID,
		VersionBindingRef:        "test-only:snapshot-sha256",
		RawResponseSampleHash:    bodyHash,
		Status:                   newsapp.AvailabilitySourceQualificationQualified,
		EffectiveAt:              effectiveAt,
	}
	qualificationCatalog, err := sourceadapter.NewStaticAvailabilityQualificationCatalog(
		[]newsapp.AvailabilitySourceQualification{qualification},
	)
	if err != nil {
		t.Fatal(err)
	}

	collect, err := newsapp.NewCollectSourceWithPolicyResolver(sourceCatalog, officialAPIMariaDBTransport{body: body}, repository)
	if err != nil {
		t.Fatal(err)
	}
	var sequence atomic.Uint64
	newID := func() string { return fmt.Sprintf("b-group-id-%d-%d", now.UnixNano(), sequence.Add(1)) }
	ingest, err := newsapp.NewIngestCollectedSourceWithProviderPolicies(repository, newID, sourceCatalog, providerCatalog)
	if err != nil {
		t.Fatal(err)
	}
	qualifier, err := newsapp.NewQualifyCollectedAvailability(qualificationCatalog)
	if err != nil {
		t.Fatal(err)
	}
	recordEvidence, err := newsapp.NewRecordAvailabilityEvidence(repository, sourceCatalog, providerCatalog, newID)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := newsapp.NewRecordCollectedAvailability(recordEvidence)
	if err != nil {
		t.Fatal(err)
	}
	diagnosedRecorder := &diagnosingAvailabilityRecorder{inner: recorder}
	reviewRule, err := newsapp.NewSourceQualifiedAvailabilityReviewRule(qualificationCatalog, reviewRuleVersion)
	if err != nil {
		t.Fatal(err)
	}
	reviewDecision, err := newsapp.NewReviewAvailabilityDecision(repository, reviewRule, newID, repository)
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := newsapp.NewReviewCollectedAvailability(reviewDecision, reviewRuleVersion)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewFeedCollectionAdapterWithAvailabilityReview(
		[]newsapp.SourceEndpointRegistration{registration}, collect, ingest,
		qualifier, []string{fieldPath}, diagnosedRecorder, reviewer,
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := adapter.Execute(ctx, "run-b-group", CollectFeedsStageCode, []string{SourceEndpointSnapshotRef(registration)})
	if err != nil {
		t.Fatalf("execute B-group production chain: %v", err)
	}
	if diagnosedRecorder.err != nil {
		t.Fatalf("G4 Record rejected the qualified candidate: %v", diagnosedRecorder.err)
	}
	if err := ValidateFeedCollectionPayload(result.Payload); err != nil {
		t.Fatalf("validate Feed payload: %v", err)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "success" || output.IngestedCount != 1 || len(output.Sources) != 1 ||
		len(output.Sources[0].NewsIDs) != 1 {
		t.Fatalf("B-group Feed output = %+v", output)
	}
	newsID := output.Sources[0].NewsIDs[0]

	var versionID, decisionID, status, selectedEvidenceID, selectedRuleVersion string
	if err := db.QueryRowContext(ctx, `SELECT version_id, decision_id, status, IFNULL(selected_evidence_id,''), review_rule_version
		FROM news_version_availability_decisions WHERE version_id IN
		(SELECT version_id FROM news_item_versions WHERE news_id = ?)`, newsID,
	).Scan(&versionID, &decisionID, &status, &selectedEvidenceID, &selectedRuleVersion); err != nil {
		t.Fatalf("read B-group availability decision: %v", err)
	}
	if status != string(domain.AvailabilityDecisionVerified) || selectedEvidenceID == "" ||
		selectedRuleVersion != reviewRuleVersion {
		t.Fatalf("decision=%s/%s status=%s selected=%q rule=%q", versionID, decisionID, status, selectedEvidenceID, selectedRuleVersion)
	}
	var evidenceStatus, precision, fieldPathInDB, sampleHash string
	if err := db.QueryRowContext(ctx, `SELECT parse_status, source_precision, field_path, IFNULL(evidence_hash,'')
		FROM news_version_availability_evidence WHERE evidence_id = ?`, selectedEvidenceID,
	).Scan(&evidenceStatus, &precision, &fieldPathInDB, &sampleHash); err != nil {
		t.Fatalf("read B-group selected evidence: %v", err)
	}
	if evidenceStatus != string(domain.AvailabilityTimeParsed) ||
		precision != string(domain.AvailabilityTimePrecisionDisclosureBoundary) ||
		fieldPathInDB != fieldPath || sampleHash != bodyHash {
		t.Fatalf("evidence=%s status=%s precision=%s field=%s hash=%s", selectedEvidenceID, evidenceStatus, precision, fieldPathInDB, sampleHash)
	}

	// The Decision's recorded_at is produced during this execution, after the
	// `now` captured at test start. A strict PIT read must use a cutoff at or
	// after that decision time; otherwise the record is correctly invisible.
	systemAsOf := time.Now().UTC().Add(time.Minute)
	pit, err := repository.GetEvidenceVersionAt(ctx, newsapp.GetEvidenceRequest{
		SystemAsOf: systemAsOf, NewsID: newsID, VersionID: versionID,
	})
	if err != nil {
		t.Fatalf("B-group strict PIT read: %v", err)
	}
	wantAvailableAt := time.Date(2026, 10, 31, 15, 59, 59, 0, time.UTC)
	if pit.AvailableAt == nil || !pit.AvailableAt.Equal(wantAvailableAt) {
		t.Fatalf("B-group available_at = %v, want %v", pit.AvailableAt, wantAvailableAt)
	}
}
