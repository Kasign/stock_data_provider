package domain

import (
	"errors"
	"testing"
)

func TestValidateNewsItemRelationPreservesCrossSourceRepost(t *testing.T) {
	relation := NewsItemRelation{
		ID: "rel-1", FromNewsID: "news-repost", FromSourceID: "source-repost",
		ToNewsID: "news-original", ToSourceID: "source-original",
		Kind: NewsItemRelationRepostOf, EvidenceObservation: "observation-1",
	}
	if err := ValidateNewsItemRelation(relation); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNewsItemRelationGraph([]NewsItemRelation{relation}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNewsItemRelationRequiresPairedVersionEvidence(t *testing.T) {
	relation := NewsItemRelation{
		ID: "rel-1", FromNewsID: "news-repost", FromSourceID: "source-repost",
		ToNewsID: "news-original", ToSourceID: "source-original",
		Kind: NewsItemRelationRepostOf, FromVersionID: "from-v1",
	}
	if !errors.Is(ValidateNewsItemRelation(relation), ErrNewsItemRelationInvalid) {
		t.Fatal("version evidence must identify both sides")
	}
	relation.EvidenceObservation = "observation-1"
	if !errors.Is(ValidateNewsItemRelation(relation), ErrNewsItemRelationInvalid) {
		t.Fatal("partial version binding must be rejected even with observation evidence")
	}
}

func TestAssessNewsItemRelationEvidenceTracksBothContentVersions(t *testing.T) {
	relation := NewsItemRelation{
		ID: "rel-1", FromNewsID: "news-repost", FromSourceID: "source-repost",
		ToNewsID: "news-original", ToSourceID: "source-original",
		Kind: NewsItemRelationRepostOf, EvidenceObservation: "obs-1",
		FromVersionID: "from-v1", ToVersionID: "to-v1",
	}
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
	relation := NewsItemRelation{
		ID: "rel-1", FromNewsID: "news-repost", FromSourceID: "source-repost",
		ToNewsID: "news-original", ToSourceID: "source-original",
		Kind: NewsItemRelationRepostOf, EvidenceObservation: "obs-1",
	}
	status, err := AssessNewsItemRelationEvidence(relation, "from-v1", "to-v1")
	if err != nil || status != NewsItemRelationEvidenceUnversioned {
		t.Fatalf("status=%q err=%v", status, err)
	}
}

func TestAssessNewsItemRelationEvidenceRequiresCurrentVersionIDs(t *testing.T) {
	relation := NewsItemRelation{
		ID: "rel-1", FromNewsID: "news-repost", FromSourceID: "source-repost",
		ToNewsID: "news-original", ToSourceID: "source-original",
		Kind: NewsItemRelationRepostOf, FromVersionID: "from-v1", ToVersionID: "to-v1",
	}
	if _, err := AssessNewsItemRelationEvidence(relation, "from-v1", ""); !errors.Is(err, ErrNewsItemRelationInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateNewsItemRelationRejectsSameSourceOrSelfRelation(t *testing.T) {
	relation := NewsItemRelation{
		ID: "rel-1", FromNewsID: "news-1", FromSourceID: "source-1",
		ToNewsID: "news-2", ToSourceID: "source-1",
		Kind: NewsItemRelationRepostOf, FromVersionID: "from-v1", ToVersionID: "to-v1",
	}
	if !errors.Is(ValidateNewsItemRelation(relation), ErrNewsItemRelationInvalid) {
		t.Fatal("same-source repost must not be classified as cross-source repost")
	}
	relation.ToSourceID = "source-2"
	relation.ToNewsID = relation.FromNewsID
	if !errors.Is(ValidateNewsItemRelation(relation), ErrNewsItemRelationInvalid) {
		t.Fatal("self relation must be rejected")
	}
}

func TestValidateNewsItemRelationGraphRejectsCyclesAndDuplicateEdges(t *testing.T) {
	a := NewsItemRelation{
		ID: "rel-a", FromNewsID: "news-a", FromSourceID: "source-a",
		ToNewsID: "news-b", ToSourceID: "source-b",
		Kind: NewsItemRelationRepostOf, EvidenceObservation: "obs-a",
	}
	b := NewsItemRelation{
		ID: "rel-b", FromNewsID: "news-b", FromSourceID: "source-b",
		ToNewsID: "news-a", ToSourceID: "source-a",
		Kind: NewsItemRelationRepostOf, EvidenceObservation: "obs-b",
	}
	if !errors.Is(ValidateNewsItemRelationGraph([]NewsItemRelation{a, b}), ErrNewsItemRelationInvalid) {
		t.Fatal("cyclic repost graph must be rejected")
	}
	b.ID = "rel-b-duplicate"
	b.FromNewsID, b.FromSourceID = a.FromNewsID, a.FromSourceID
	b.ToNewsID, b.ToSourceID = a.ToNewsID, a.ToSourceID
	if !errors.Is(ValidateNewsItemRelationGraph([]NewsItemRelation{a, b}), ErrNewsItemRelationInvalid) {
		t.Fatal("duplicate repost edge must be rejected")
	}
}
