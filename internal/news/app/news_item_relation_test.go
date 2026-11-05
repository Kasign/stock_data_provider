package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type memoryNewsRelationRepository struct {
	observations map[string]domain.Observation
	versions     map[string]domain.NewsItemVersion
	relations    map[string][]domain.NewsItemRelation
	failAppend   bool
}

type memoryNewsRelationTx struct {
	data *memoryNewsRelationRepository
}

func (r *memoryNewsRelationRepository) WithinNewsItemRelationTransaction(ctx context.Context, fn func(NewsItemRelationTransaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	working := &memoryNewsRelationRepository{
		observations: map[string]domain.Observation{},
		versions:     map[string]domain.NewsItemVersion{},
		relations:    map[string][]domain.NewsItemRelation{},
		failAppend:   r.failAppend,
	}
	for k, v := range r.observations {
		working.observations[k] = v
	}
	for k, v := range r.versions {
		working.versions[k] = v
	}
	for k, v := range r.relations {
		working.relations[k] = append([]domain.NewsItemRelation(nil), v...)
	}
	if err := fn(memoryNewsRelationTx{data: working}); err != nil {
		return err
	}
	*r = *working
	return nil
}

func (tx memoryNewsRelationTx) ObservationByID(_ context.Context, id string) (domain.Observation, bool, error) {
	v, ok := tx.data.observations[id]
	return v, ok, nil
}

func (tx memoryNewsRelationTx) NewsItemVersionByID(_ context.Context, id string) (domain.NewsItemVersion, bool, error) {
	v, ok := tx.data.versions[id]
	return v, ok, nil
}

func (tx memoryNewsRelationTx) LatestNewsItemVersion(_ context.Context, newsID string) (domain.NewsItemVersion, bool, error) {
	var latest domain.NewsItemVersion
	for _, v := range tx.data.versions {
		if v.NewsID == newsID && v.VersionNo > latest.VersionNo {
			latest = v
		}
	}
	return latest, latest.ID != "", nil
}

func (tx memoryNewsRelationTx) ListAllNewsItemRelations(_ context.Context) ([]domain.NewsItemRelation, error) {
	var all []domain.NewsItemRelation
	for _, items := range tx.data.relations {
		all = append(all, items...)
	}
	return all, nil
}

func (tx memoryNewsRelationTx) AppendNewsItemRelation(_ context.Context, relation domain.NewsItemRelation) error {
	if tx.data.failAppend {
		return errors.New("append failed")
	}
	tx.data.relations[relation.FromNewsID] = append(tx.data.relations[relation.FromNewsID], relation)
	return nil
}

func relationAppFixture() (*memoryNewsRelationRepository, *ApplyNewsItemRelation) {
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	r := &memoryNewsRelationRepository{
		observations: map[string]domain.Observation{
			"from-obs":    {ID: "from-obs", NewsID: "news-from", SourceID: "source-from"},
			"to-obs":      {ID: "to-obs", NewsID: "news-to", SourceID: "source-to"},
			"foreign-obs": {ID: "foreign-obs", NewsID: "news-foreign", SourceID: "source-from"},
		},
		versions: map[string]domain.NewsItemVersion{
			"from-v1":    {ID: "from-v1", NewsID: "news-from", VersionNo: 1, ObservationID: "from-obs"},
			"to-v1":      {ID: "to-v1", NewsID: "news-to", VersionNo: 1, ObservationID: "to-obs"},
			"foreign-v1": {ID: "foreign-v1", NewsID: "news-foreign", VersionNo: 1, ObservationID: "foreign-obs"},
		},
		relations: map[string][]domain.NewsItemRelation{},
	}
	u := NewApplyNewsItemRelation(r, func() string { return "generated-id" })
	u.now = func() time.Time { return now }
	return r, u
}

func relationAppRequest() NewsItemRelationRequest {
	return NewsItemRelationRequest{Relation: domain.NewsItemRelation{
		FromNewsID:         "news-from",
		FromSourceID:       "source-from",
		ToNewsID:           "news-to",
		ToSourceID:         "source-to",
		Kind:               domain.NewsItemRelationRepostOf,
		Status:             domain.NewsItemRelationConfirmed,
		EvidenceObservation: "from-obs",
		FromVersionID:      "from-v1",
		ToVersionID:        "to-v1",
	}}
}

func TestApplyNewsItemRelationValidatesOwnershipAndAppends(t *testing.T) {
	r, u := relationAppFixture()
	got, err := u.Execute(context.Background(), relationAppRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "generated-id" || len(r.relations["news-from"]) != 1 {
		t.Fatalf("got=%+v stored=%+v", got, r.relations)
	}
}

func TestApplyNewsItemRelationRejectsForeignObservationAndRollsBack(t *testing.T) {
	r, u := relationAppFixture()
	req := relationAppRequest()
	req.Relation.EvidenceObservation = "foreign-obs"
	if _, err := u.Execute(context.Background(), req); !errors.Is(err, ErrNewsItemRelationTransaction) {
		t.Fatalf("err=%v", err)
	}
	if len(r.relations["news-from"]) != 0 {
		t.Fatal("foreign evidence must not append")
	}
}

func TestApplyNewsItemRelationRejectsForeignVersionAndRollsBack(t *testing.T) {
	r, u := relationAppFixture()
	req := relationAppRequest()
	req.Relation.ToVersionID = "foreign-v1"
	if _, err := u.Execute(context.Background(), req); !errors.Is(err, ErrNewsItemRelationTransaction) {
		t.Fatalf("err=%v", err)
	}
	if len(r.relations["news-from"]) != 0 {
		t.Fatal("foreign version must not append")
	}
}

func TestApplyNewsItemRelationRejectsDuplicateEdgeAndCycle(t *testing.T) {
	r, u := relationAppFixture()
	req := relationAppRequest()
	if _, err := u.Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	duplicate := relationAppRequest()
	duplicate.Relation.ID = "duplicate"
	if _, err := u.Execute(context.Background(), duplicate); !errors.Is(err, ErrNewsItemRelationTransaction) {
		t.Fatalf("duplicate err=%v", err)
	}

	cycle := relationAppRequest()
	cycle.Relation.ID = "cycle"
	cycle.Relation.FromNewsID = "news-to"
	cycle.Relation.FromSourceID = "source-to"
	cycle.Relation.ToNewsID = "news-from"
	cycle.Relation.ToSourceID = "source-from"
	cycle.Relation.EvidenceObservation = "to-obs"
	cycle.Relation.FromVersionID = "to-v1"
	cycle.Relation.ToVersionID = "from-v1"
	if _, err := u.Execute(context.Background(), cycle); !errors.Is(err, ErrNewsItemRelationTransaction) {
		t.Fatalf("cycle err=%v", err)
	}
}

func TestApplyNewsItemRelationSupportsProposedToConfirmedReplacement(t *testing.T) {
	r, u := relationAppFixture()
	old := relationAppRequest().Relation
	old.ID = "rel-old"
	r.relations["news-from"] = []domain.NewsItemRelation{old}

	proposal := relationAppRequest()
	proposal.Relation.Status = domain.NewsItemRelationProposed
	proposal.Relation.SupersedesRelationID = old.ID
	proposal.Relation.ID = "rel-proposal"
	if _, err := u.Execute(context.Background(), proposal); err != nil {
		t.Fatalf("append proposal: %v", err)
	}

	confirmed := relationAppRequest()
	confirmed.Relation.SupersedesRelationID = proposal.Relation.ID
	confirmed.Relation.ID = "rel-confirmed"
	if _, err := u.Execute(context.Background(), confirmed); err != nil {
		t.Fatalf("confirm proposal: %v", err)
	}
	if got := len(r.relations["news-from"]); got != 3 {
		t.Fatalf("expected immutable old/proposed/confirmed history, got %d records", got)
	}
}

func TestApplyNewsItemRelationRechecksStaleEvidenceWithNewVersions(t *testing.T) {
	r, u := relationAppFixture()
	old := relationAppRequest().Relation
	old.ID = "rel-stale"
	r.relations["news-from"] = []domain.NewsItemRelation{old}
	r.versions["from-v2"] = domain.NewsItemVersion{
		ID: "from-v2", NewsID: "news-from", VersionNo: 2, ObservationID: "from-obs",
	}
	r.versions["to-v2"] = domain.NewsItemVersion{
		ID: "to-v2", NewsID: "news-to", VersionNo: 2, ObservationID: "to-obs",
	}

	recheck := relationAppRequest()
	recheck.Relation.ID = "rel-rechecked"
	recheck.Relation.SupersedesRelationID = old.ID
	recheck.Relation.FromVersionID = "from-v2"
	recheck.Relation.ToVersionID = "to-v2"
	if _, err := u.Execute(context.Background(), recheck); err != nil {
		t.Fatalf("append rechecked assertion: %v", err)
	}
	if got := len(r.relations["news-from"]); got != 2 {
		t.Fatalf("expected stale and rechecked assertions to remain, got %d records", got)
	}
	if r.relations["news-from"][0].FromVersionID != "from-v1" ||
		r.relations["news-from"][1].FromVersionID != "from-v2" {
		t.Fatal("recheck must preserve old evidence and append new version evidence")
	}
}

func TestApplyNewsItemRelationAppendFailureRollsBack(t *testing.T) {
	r, u := relationAppFixture()
	r.failAppend = true
	if _, err := u.Execute(context.Background(), relationAppRequest()); !errors.Is(err, ErrNewsItemRelationTransaction) {
		t.Fatalf("err=%v", err)
	}
	if len(r.relations["news-from"]) != 0 {
		t.Fatal("append failure must rollback")
	}
}

func TestApplyNewsItemRelationRequiresPairedVersionIDs(t *testing.T) {
	_, u := relationAppFixture()
	req := relationAppRequest()
	req.Relation.ToVersionID = ""
	if _, err := u.Execute(context.Background(), req); !errors.Is(err, domain.ErrNewsItemRelationInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestApplyNewsItemRelationRejectsStaleVersionEvidence(t *testing.T) {
	r, u := relationAppFixture()
	r.versions["from-v2"] = domain.NewsItemVersion{
		ID: "from-v2", NewsID: "news-from", VersionNo: 2, ObservationID: "from-obs",
	}
	if _, err := u.Execute(context.Background(), relationAppRequest()); !errors.Is(err, ErrNewsItemRelationTransaction) {
		t.Fatalf("stale version err=%v", err)
	}
	if len(r.relations["news-from"]) != 0 {
		t.Fatal("stale version evidence must not append")
	}
}
