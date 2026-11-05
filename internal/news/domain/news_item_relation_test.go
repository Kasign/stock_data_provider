package domain

import (
	"errors"
	"testing"
	"time"
)

func relationFixture(id, from, to string) NewsItemRelation {
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	return NewsItemRelation{
		ID: id, FromNewsID: from, FromSourceID: "source-" + from,
		ToNewsID: to, ToSourceID: "source-" + to,
		Kind: NewsItemRelationRepostOf, Status: NewsItemRelationConfirmed,
		EvidenceObservation: "obs-" + id, AssertedAt: now, ObservedAt: now,
	}
}

func TestValidateNewsItemRelationPreservesCrossSourceRepost(t *testing.T) {
	relation := relationFixture("rel-1", "news-repost", "news-original")
	if err := ValidateNewsItemRelation(relation); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNewsItemRelationGraph([]NewsItemRelation{relation}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNewsItemRelationRequiresPairedVersionEvidence(t *testing.T) {
	relation := relationFixture("rel-1", "news-repost", "news-original")
	relation.FromVersionID = "from-v1"
	if !errors.Is(ValidateNewsItemRelation(relation), ErrNewsItemRelationInvalid) {
		t.Fatal("version evidence must identify both sides")
	}
	relation.EvidenceObservation = "observation-1"
	if !errors.Is(ValidateNewsItemRelation(relation), ErrNewsItemRelationInvalid) {
		t.Fatal("partial version binding must be rejected even with observation evidence")
	}
}

func TestAssessNewsItemRelationEvidenceTracksBothContentVersions(t *testing.T) {
	relation := relationFixture("rel-1", "news-repost", "news-original")
	relation.FromVersionID, relation.ToVersionID = "from-v1", "to-v1"
	status, err := AssessNewsItemRelationEvidence(relation, "from-v1", "to-v1")
	if err != nil || status != NewsItemRelationEvidenceCurrent {
		t.Fatalf("status=%q err=%v", status, err)
	}
	status, err = AssessNewsItemRelationEvidence(relation, "from-v2", "to-v1")
	if err != nil || status != NewsItemRelationEvidenceStale {
		t.Fatalf("from revision status=%q err=%v", status, err)
	}
	status, err = AssessNewsItemRelationEvidence(relation, "from-v1", "to-v2")
	if err != nil || status != NewsItemRelationEvidenceStale {
		t.Fatalf("to revision status=%q err=%v", status, err)
	}
	if relation.FromVersionID != "from-v1" || relation.ToVersionID != "to-v1" {
		t.Fatal("assessment must not rewrite historical evidence references")
	}
}

func TestAssessNewsItemRelationEvidenceMarksObservationOnlyAsUnversioned(t *testing.T) {
	relation := relationFixture("rel-1", "news-repost", "news-original")
	status, err := AssessNewsItemRelationEvidence(relation, "from-v1", "to-v1")
	if err != nil || status != NewsItemRelationEvidenceUnversioned {
		t.Fatalf("status=%q err=%v", status, err)
	}
}

func TestAssessNewsItemRelationEvidenceRequiresCurrentVersionIDs(t *testing.T) {
	relation := relationFixture("rel-1", "news-repost", "news-original")
	relation.FromVersionID, relation.ToVersionID = "from-v1", "to-v1"
	if _, err := AssessNewsItemRelationEvidence(relation, "from-v1", ""); !errors.Is(err, ErrNewsItemRelationInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateNewsItemRelationRejectsSameSourceOrSelfRelation(t *testing.T) {
	relation := relationFixture("rel-1", "news-1", "news-2")
	relation.ToSourceID = relation.FromSourceID
	if !errors.Is(ValidateNewsItemRelation(relation), ErrNewsItemRelationInvalid) {
		t.Fatal("same-source repost must not be classified as cross-source repost")
	}
	relation.ToSourceID = "source-2"
	relation.ToNewsID = relation.FromNewsID
	if !errors.Is(ValidateNewsItemRelation(relation), ErrNewsItemRelationInvalid) {
		t.Fatal("self relation must be rejected")
	}
}

func TestValidateNewsItemRelationGraphAllowsMultipleOriginalItems(t *testing.T) {
	a := relationFixture("rel-a", "news-repost", "news-original-a")
	b := relationFixture("rel-b", "news-repost", "news-original-b")
	if err := ValidateNewsItemRelationGraph([]NewsItemRelation{a, b}); err != nil {
		t.Fatalf("one repost item may reference multiple originals: %v", err)
	}
}

func TestValidateNewsItemRelationGraphRejectsCyclesAndDuplicateEdges(t *testing.T) {
	a := relationFixture("rel-a", "news-a", "news-b")
	b := relationFixture("rel-b", "news-b", "news-a")
	if !errors.Is(ValidateNewsItemRelationGraph([]NewsItemRelation{a, b}), ErrNewsItemRelationInvalid) {
		t.Fatal("cyclic repost graph must be rejected")
	}
	b = relationFixture("rel-b-duplicate", "news-a", "news-b")
	if !errors.Is(ValidateNewsItemRelationGraph([]NewsItemRelation{a, b}), ErrNewsItemRelationInvalid) {
		t.Fatal("duplicate repost edge must be rejected")
	}
}

func TestValidateNewsItemRelationHistoryRequiresAppendOnlyReplacement(t *testing.T) {
	old := relationFixture("rel-old", "news-repost", "news-original")
	next := relationFixture("rel-new", "news-repost", "news-original")
	next.SupersedesRelationID = old.ID
	next.EvidenceObservation = "obs-rechecked"
	if err := ValidateNewsItemRelationHistory([]NewsItemRelation{old, next}); err != nil {
		t.Fatal(err)
	}

	retracted := relationFixture("rel-retracted", "news-repost", "news-original")
	retracted.Status = NewsItemRelationRetracted
	retracted.SupersedesRelationID = old.ID
	if err := ValidateNewsItemRelationHistory([]NewsItemRelation{old, retracted}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNewsItemRelationHistoryConfirmedDescendantReplacesProposedChain(t *testing.T) {
	old := relationFixture("rel-old", "news-repost", "news-original")
	proposal := relationFixture("rel-proposal", "news-repost", "news-original")
	proposal.Status = NewsItemRelationProposed
	proposal.SupersedesRelationID = old.ID
	confirmed := relationFixture("rel-confirmed", "news-repost", "news-original")
	confirmed.SupersedesRelationID = proposal.ID

	if err := ValidateNewsItemRelationHistory([]NewsItemRelation{old, proposal, confirmed}); err != nil {
		t.Fatalf("confirmed descendant must replace the old confirmed edge through proposed state: %v", err)
	}
}

func TestValidateNewsItemRelationHistoryRejectsMissingParentAndMultipleSuccessors(t *testing.T) {
	orphan := relationFixture("rel-orphan", "news-repost", "news-original")
	orphan.SupersedesRelationID = "missing"
	if !errors.Is(ValidateNewsItemRelationHistory([]NewsItemRelation{orphan}), ErrNewsItemRelationInvalid) {
		t.Fatal("missing superseded assertion must be rejected")
	}
	old := relationFixture("rel-old", "news-repost", "news-original")
	first := relationFixture("rel-first", "news-repost", "news-original")
	first.SupersedesRelationID = old.ID
	second := relationFixture("rel-second", "news-repost", "news-original")
	second.SupersedesRelationID = old.ID
	if !errors.Is(ValidateNewsItemRelationHistory([]NewsItemRelation{old, first, second}), ErrNewsItemRelationInvalid) {
		t.Fatal("an assertion must have at most one direct successor")
	}
}

func TestValidateNewsItemRelationHistoryProposedReplacementDoesNotWithdrawCurrentEdge(t *testing.T) {
	old := relationFixture("rel-old", "news-repost", "news-original-a")
	proposal := relationFixture("rel-proposal", "news-repost", "news-original-b")
	proposal.Status = NewsItemRelationProposed
	proposal.SupersedesRelationID = old.ID
	if err := ValidateNewsItemRelationHistory([]NewsItemRelation{old, proposal}); err != nil {
		t.Fatal(err)
	}
	// proposed 尚未确认，旧边仍然是当前有效关系；不能把同一旧断言的替代关系提前生效。
	confirmed := relationFixture("rel-confirmed", "news-repost", "news-original-b")
	confirmed.SupersedesRelationID = old.ID
	if !errors.Is(ValidateNewsItemRelationHistory([]NewsItemRelation{old, proposal, confirmed}), ErrNewsItemRelationInvalid) {
		t.Fatal("one historical assertion cannot have multiple direct successors")
	}
}
