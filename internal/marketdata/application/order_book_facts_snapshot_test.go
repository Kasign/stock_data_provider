package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/snapshotport"
	"example.com/stock-analysis-flow/internal/platform/security"
)

type orderBookReaderStub struct {
	book *model.OrderBook
	err  error
}

func (stub orderBookReaderStub) FetchOrderBook(context.Context, string) (*model.OrderBook, error) {
	return stub.book, stub.err
}

type orderBookSnapshotRepositoryStub struct {
	snapshot    app.OrderBookFactsSnapshot
	saved       bool
	reusable    app.OrderBookFactsSnapshot
	lookupCalls int
}

func (stub *orderBookSnapshotRepositoryStub) SaveOrderBookFactsSnapshot(_ context.Context, snapshot app.OrderBookFactsSnapshot) error {
	stub.snapshot = snapshot
	stub.saved = true
	return nil
}

func (stub *orderBookSnapshotRepositoryStub) GetOrderBookFactsSnapshot(context.Context, string) (app.OrderBookFactsSnapshot, error) {
	if !stub.saved {
		return app.OrderBookFactsSnapshot{}, errors.New("not found")
	}
	return stub.snapshot, nil
}

func (stub *orderBookSnapshotRepositoryStub) GetOrderBookFactsSnapshotForSystemAsOf(
	_ context.Context, _ string, _ string, _ time.Time,
) (app.OrderBookFactsSnapshot, error) {
	stub.lookupCalls++
	if stub.reusable.Manifest.SnapshotRef == "" {
		return app.OrderBookFactsSnapshot{}, snapshotport.ErrOrderBookFactsSnapshotNotFound
	}
	return stub.reusable, nil
}

func (stub *orderBookSnapshotRepositoryStub) GetLatestOrderBookFactsSnapshotAtOrBefore(
	ctx context.Context, symbol, purpose string, asOf time.Time,
) (app.OrderBookFactsSnapshot, error) {
	return stub.GetOrderBookFactsSnapshotForSystemAsOf(ctx, symbol, purpose, asOf)
}

type orderBookPolicyStoreStub struct {
	current authorization.CurrentPolicy
}

func (stub *orderBookPolicyStoreStub) PublishDataAuthorizationPolicyVersion(
	context.Context, string, authorization.PolicyVersion, app.DataAuthorizationPolicyPublicationAudit,
) error {
	return errors.New("not used")
}

func (stub *orderBookPolicyStoreStub) ResolveCurrentDataAuthorizationPolicy(context.Context) (authorization.CurrentPolicy, error) {
	return stub.current, nil
}

func (stub *orderBookPolicyStoreStub) ResolveCurrentDataAuthorizationPolicyByRef(_ context.Context, ref string) (authorization.CurrentPolicy, error) {
	if ref != stub.current.Version.PolicyRef {
		return authorization.CurrentPolicy{}, errors.New("policy ref mismatch")
	}
	return stub.current, nil
}

type orderBookAuthorizedRepositoryStub struct {
	decisionStore *dataAuthorizationDecisionStoreFake
	snapshot      app.OrderBookFactsSnapshot
	current       authorization.CurrentPolicy
}

func (stub *orderBookAuthorizedRepositoryStub) ReadDataAuthorizedOrderBookFactsSnapshot(
	_ context.Context, authorizationRef, snapshotRef string,
) (app.DataAuthorizationDecision, app.OrderBookFactsSnapshot, authorization.CurrentPolicy, error) {
	decision, err := stub.decisionStore.GetDataAuthorizationDecision(context.Background(), authorizationRef)
	if err != nil {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, err
	}
	if stub.snapshot.Manifest.SnapshotRef != snapshotRef {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, errors.New("snapshot ref mismatch")
	}
	return decision, stub.snapshot, stub.current, nil
}

func orderBookApplicationSnapshot(t *testing.T) (app.OrderBookFactsSnapshot, *model.OrderBook) {
	t.Helper()
	book := &model.OrderBook{
		Symbol: "600519.SH",
		Bids: []model.OrderBookLevel{
			{Price: 1313.10, Volume: 1200}, {Price: 1313.00, Volume: 3500},
			{Price: 1312.90, Volume: 4800}, {Price: 1312.80, Volume: 6100},
			{Price: 1312.70, Volume: 7400},
		},
		Asks: []model.OrderBookLevel{
			{Price: 1313.20, Volume: 900}, {Price: 1313.30, Volume: 1700},
			{Price: 1313.40, Volume: 2500}, {Price: 1313.50, Volume: 3300},
			{Price: 1313.60, Volume: 4100},
		},
		ObservedAt:       time.Date(2026, 11, 27, 10, 15, 0, 0, time.UTC),
		SystemObservedAt: time.Date(2026, 11, 27, 10, 15, 2, 0, time.UTC),
		Source:           "eastmoney", QualityStatus: model.QualityValid,
	}
	payload, err := app.BuildOrderBookFactsPayload(*book)
	if err != nil {
		t.Fatal(err)
	}
	sources := []app.DailyBarsCloseSeriesSource{{SourceRef: "eastmoney", SourceVersion: orderBookStringPointer("order-book-v1")}}
	snapshot, err := app.BuildOrderBookFactsSnapshot(payload, sources, "principal:service:quant", "risk_review", book.SystemObservedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, book
}

func newOrderBookTestOwner(t *testing.T, now time.Time, policyRef string) (*DataAuthorizationOwner, *orderBookPolicyStoreStub, *dataAuthorizationDecisionStoreFake) {
	t.Helper()
	policy := authorization.DataAccessPolicy{
		SchemaVersion:   authorization.DataAccessPolicySchemaVersion,
		Status:          authorization.PolicyApproved,
		AllowedSubjects: []string{"principal:service:quant"},
		AllowedPurposes: []string{"risk_review"},
		RequiredScopes:  []string{"data:read"},
		EffectiveAt:     now.Add(-time.Minute).UTC(),
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	version := authorization.PolicyVersion{
		PolicyType: authorization.DataAccessPolicyType, PolicyRef: policyRef,
		PolicyVersion: "order-book-policy-v1", Payload: payload,
		PayloadHash: hex.EncodeToString(sum[:]), CreatedAt: now.Add(-time.Minute).UTC(),
	}
	policyStore := &orderBookPolicyStoreStub{current: authorization.CurrentPolicy{Version: version, Policy: policy}}
	decisionStore := &dataAuthorizationDecisionStoreFake{decisions: map[string]app.DataAuthorizationDecision{}, revoked: map[string]bool{}}
	owner, err := NewDataAuthorizationOwner(policyStore, decisionStore)
	if err != nil {
		t.Fatal(err)
	}
	owner.now = func() time.Time { return now.UTC() }
	return owner, policyStore, decisionStore
}

func orderBookPrincipalContext(t *testing.T) context.Context {
	t.Helper()
	ctx, err := security.WithPrincipal(context.Background(), security.Principal{
		Kind: security.IdentityService, AuthMethod: security.AuthMethodServiceBearer,
		Subject: "quant", Scopes: []string{security.ScopeDataRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestOrderBookFactsSnapshotCollectorPersistsWithoutAuthorization(t *testing.T) {
	_, book := orderBookApplicationSnapshot(t)
	repository := &orderBookSnapshotRepositoryStub{}
	collector, err := NewOrderBookFactsSnapshotCollector(orderBookReaderStub{book: book}, repository)
	if err != nil {
		t.Fatal(err)
	}
	// system_as_of 零值表示以实际观测时刻作为快照时点，适合 Run 前预采集。
	ref, err := collector.Persist(orderBookPrincipalContext(t), "600519.SH", time.Time{}, "risk_review")
	if err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	if !repository.saved || ref == "" || ref != repository.snapshot.Manifest.SnapshotRef {
		t.Fatalf("Persist() did not persist the snapshot: saved=%v ref=%s stored=%+v",
			repository.saved, ref, repository.snapshot.Manifest)
	}
	wantAsOf := book.SystemObservedAt.UTC().Format(time.RFC3339Nano)
	if repository.snapshot.Manifest.SystemAsOf != wantAsOf {
		t.Fatalf("snapshot system_as_of = %s, want observation instant %s",
			repository.snapshot.Manifest.SystemAsOf, wantAsOf)
	}
}

func TestOrderBookFactsSnapshotCollectorReusesAsOfSnapshotWithoutProvider(t *testing.T) {
	snapshot, _ := orderBookApplicationSnapshot(t)
	publishedAt, err := time.Parse(time.RFC3339Nano, snapshot.Manifest.SystemAsOf)
	if err != nil {
		t.Fatal(err)
	}
	repository := &orderBookSnapshotRepositoryStub{reusable: snapshot}
	collector, err := NewOrderBookFactsSnapshotCollector(
		orderBookReaderStub{err: errors.New("live provider must not be called on as-of reuse")}, repository,
	)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := collector.Persist(orderBookPrincipalContext(t), "600519.SH", publishedAt.Add(time.Minute), "risk_review")
	if err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	if ref != snapshot.Manifest.SnapshotRef {
		t.Fatalf("reused ref = %q, want %q", ref, snapshot.Manifest.SnapshotRef)
	}
	if repository.lookupCalls != 1 || repository.saved {
		t.Fatalf("reuse lookup_calls=%d saved=%v, want one lookup and no new snapshot", repository.lookupCalls, repository.saved)
	}
}

func TestOrderBookFactsSnapshotCollectorRejectsObservationLaterThanAsOf(t *testing.T) {
	_, book := orderBookApplicationSnapshot(t)
	collector, err := NewOrderBookFactsSnapshotCollector(
		orderBookReaderStub{book: book}, &orderBookSnapshotRepositoryStub{},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = collector.Persist(
		orderBookPrincipalContext(t), "600519.SH", book.SystemObservedAt.Add(-time.Second), "risk_review",
	)
	if !errors.Is(err, ErrOrderBookFactsSnapshotStaleAsOf) {
		t.Fatalf("error = %v, want ErrOrderBookFactsSnapshotStaleAsOf", err)
	}
}

func TestOrderBookFactsSnapshotCollectorRejectsNonUTCSystemAsOf(t *testing.T) {
	_, book := orderBookApplicationSnapshot(t)
	collector, err := NewOrderBookFactsSnapshotCollector(
		orderBookReaderStub{book: book}, &orderBookSnapshotRepositoryStub{},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = collector.Persist(orderBookPrincipalContext(t), "600519.SH", book.SystemObservedAt.In(time.FixedZone("CST", 8*3600)), "risk_review")
	if err == nil {
		t.Fatal("non-UTC system_as_of was accepted")
	}
}

func TestOrderBookFactsSnapshotCreatorPersistsAndAuthorizes(t *testing.T) {
	_, book := orderBookApplicationSnapshot(t)
	systemAsOf := book.SystemObservedAt.Add(time.Second)
	owner, _, _ := newOrderBookTestOwner(t, systemAsOf, authorization.OrderBookFactsPolicyRef)
	repository := &orderBookSnapshotRepositoryStub{}
	creator, err := NewOrderBookFactsSnapshotCreator(orderBookReaderStub{book: book}, repository, owner)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := creator.Create(orderBookPrincipalContext(t), "600519.SH", systemAsOf, "risk_review")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !repository.saved || refs.SnapshotRef == "" || refs.AuthorizationRef == "" {
		t.Fatalf("Create() did not persist and authorize: saved=%v refs=%+v", repository.saved, refs)
	}
	if repository.snapshot.Manifest.SnapshotRef != refs.SnapshotRef ||
		repository.snapshot.Manifest.PurposeCode != "risk_review" {
		t.Fatalf("stored snapshot does not match returned references: %+v", repository.snapshot.Manifest)
	}
}

func TestOrderBookFactsSnapshotCreatorReusesExactSystemAsOfSnapshot(t *testing.T) {
	snapshot, book := orderBookApplicationSnapshot(t)
	systemAsOf := book.SystemObservedAt.Add(time.Second)
	owner, _, _ := newOrderBookTestOwner(t, systemAsOf, authorization.OrderBookFactsPolicyRef)
	repository := &orderBookSnapshotRepositoryStub{reusable: snapshot}
	creator, err := NewOrderBookFactsSnapshotCreator(
		orderBookReaderStub{err: errors.New("live provider must not be called on exact reuse")}, repository, owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := creator.Create(orderBookPrincipalContext(t), "600519.SH", systemAsOf, "risk_review")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if refs.SnapshotRef != snapshot.Manifest.SnapshotRef || refs.AuthorizationRef == "" {
		t.Fatalf("reuse refs = %+v, want existing snapshot ref and a new authorization", refs)
	}
	if repository.lookupCalls != 1 {
		t.Fatalf("lookup calls = %d, want 1", repository.lookupCalls)
	}
	if repository.saved {
		t.Fatal("exact reuse must not persist a duplicate snapshot")
	}
}

func TestOrderBookFactsSnapshotCreatorReusesEarlierSnapshot(t *testing.T) {
	snapshot, book := orderBookApplicationSnapshot(t)
	snapshotAsOf, err := time.Parse(time.RFC3339Nano, snapshot.Manifest.SystemAsOf)
	if err != nil {
		t.Fatal(err)
	}
	laterAsOf := snapshotAsOf.Add(time.Minute)
	owner, _, _ := newOrderBookTestOwner(t, laterAsOf, authorization.OrderBookFactsPolicyRef)
	repository := &orderBookSnapshotRepositoryStub{reusable: snapshot}
	creator, err := NewOrderBookFactsSnapshotCreator(
		orderBookReaderStub{err: errors.New("live provider must not be called on as-of reuse")}, repository, owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := creator.Create(orderBookPrincipalContext(t), "600519.SH", laterAsOf, "risk_review")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if refs.SnapshotRef != snapshot.Manifest.SnapshotRef || refs.AuthorizationRef == "" {
		t.Fatalf("as-of reuse refs = %+v, want existing snapshot ref and a new authorization", refs)
	}
	if book.SystemObservedAt.After(snapshotAsOf) {
		t.Fatalf("fixture observation %s is later than snapshot as-of %s", book.SystemObservedAt, snapshotAsOf)
	}
}

func TestOrderBookFactsSnapshotCreatorRejectsReusedSnapshotPublishedLaterThanRequest(t *testing.T) {
	snapshot, book := orderBookApplicationSnapshot(t)
	systemAsOf := book.SystemObservedAt // 早于快照发布时点 1 秒
	owner, _, _ := newOrderBookTestOwner(t, systemAsOf.Add(time.Minute), authorization.OrderBookFactsPolicyRef)
	repository := &orderBookSnapshotRepositoryStub{reusable: snapshot}
	creator, err := NewOrderBookFactsSnapshotCreator(
		orderBookReaderStub{err: errors.New("live provider must not be called on invalid reuse")}, repository, owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creator.Create(orderBookPrincipalContext(t), "600519.SH", systemAsOf, "risk_review"); err == nil {
		t.Fatal("creator accepted a reused snapshot published after the requested system_as_of")
	}
}

func TestOrderBookFactsSnapshotCreatorRejectsStaleObservation(t *testing.T) {
	_, book := orderBookApplicationSnapshot(t)
	owner, _, _ := newOrderBookTestOwner(t, book.SystemObservedAt, authorization.OrderBookFactsPolicyRef)
	creator, err := NewOrderBookFactsSnapshotCreator(orderBookReaderStub{book: book}, &orderBookSnapshotRepositoryStub{}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creator.Create(orderBookPrincipalContext(t), "600519.SH", book.SystemObservedAt.Add(-time.Second), "risk_review"); err == nil {
		t.Fatal("creator accepted a system_as_of earlier than the source observation")
	}
}

func TestAuthorizedOrderBookFactsSnapshotReaderValidatesPolicyClass(t *testing.T) {
	snapshot, book := orderBookApplicationSnapshot(t)
	systemAsOf := book.SystemObservedAt.Add(time.Second)
	owner, policyStore, decisionStore := newOrderBookTestOwner(t, systemAsOf, authorization.OrderBookFactsPolicyRef)
	authRef, err := owner.IssueFor(orderBookPrincipalContext(t), snapshot.Manifest.SnapshotRef, "risk_review", authorization.OrderBookFactsPolicyRef)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewAuthorizedOrderBookFactsSnapshotReader(&orderBookAuthorizedRepositoryStub{
		decisionStore: decisionStore, snapshot: snapshot, current: policyStore.current,
	})
	if err != nil {
		t.Fatal(err)
	}
	reader.now = func() time.Time { return systemAsOf }
	loaded, err := reader.Read(orderBookPrincipalContext(t), snapshot.Manifest.SnapshotRef, authRef, "risk_review")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if loaded.Manifest.SnapshotRef != snapshot.Manifest.SnapshotRef {
		t.Fatalf("reader returned the wrong snapshot: %+v", loaded.Manifest)
	}
	wrongClass := policyStore.current
	wrongClass.Version.PolicyRef = authorization.SecurityStatusFactsPolicyRef
	wrongReader, err := NewAuthorizedOrderBookFactsSnapshotReader(&orderBookAuthorizedRepositoryStub{
		decisionStore: decisionStore, snapshot: snapshot, current: wrongClass,
	})
	if err != nil {
		t.Fatal(err)
	}
	wrongReader.now = func() time.Time { return systemAsOf }
	if _, err := wrongReader.Read(orderBookPrincipalContext(t), snapshot.Manifest.SnapshotRef, authRef, "risk_review"); err == nil {
		t.Fatal("reader accepted a different data-class policy")
	}
}

func orderBookStringPointer(value string) *string { return &value }
