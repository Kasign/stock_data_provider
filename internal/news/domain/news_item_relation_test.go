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

func TestValidateNewsItemRelationRejectsSameSourceOrSelfRelation(t *testing.T) {
	relation := NewsItemRelation{
		ID: "rel-1", FromNewsID: "news-1", FromSourceID: "source-1",
		ToNewsID: "news-2", ToSourceID: "source-1",
		Kind: NewsItemRelationRepostOf, EvidenceVersion: "version-1",
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
