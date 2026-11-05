package domain

import (
	"errors"
	"testing"
	"time"
)

func validSecurityRelation(id, status string) RelationAssertion {
	return RelationAssertion{
		ID: id, NewsID: "news-1", SubjectType: RelationSubjectSecurity,
		Security: &SecurityRelationSubject{Market: "XSHG", Symbol: "600000", Name: "浦发银行"},
		Kind:     RelationMentions, Method: RelationSourceExplicit, Status: RelationStatus(status),
		ObservedAt: time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC),
	}
}

func TestValidateRelationAssertionRequiresMarketAndSymbol(t *testing.T) {
	assertion := validSecurityRelation("r1", string(RelationConfirmed))
	assertion.Security.Market = ""
	if !errors.Is(ValidateRelationAssertion(assertion), ErrRelationInvalid) {
		t.Fatal("security relation must require market and symbol")
	}
}

func TestValidateRelationAssertionKeepsModelOutputProposed(t *testing.T) {
	assertion := validSecurityRelation("r1", string(RelationConfirmed))
	assertion.Method = RelationModel
	if !errors.Is(ValidateRelationAssertion(assertion), ErrRelationInvalid) {
		t.Fatal("model inference must not become confirmed directly")
	}
}

func TestValidateRelationEvidenceMatchesMethod(t *testing.T) {
	assertion := validSecurityRelation("r1", string(RelationConfirmed))
	valid := RelationEvidence{
		RelationID: "r1", Type: RelationEvidenceSourceField,
		ObservationID: "observation-1", FieldPath: "related_securities",
	}
	if err := ValidateRelationEvidence(assertion, valid); err != nil {
		t.Fatal(err)
	}
	valid.Type = RelationEvidenceModelOutput
	if !errors.Is(ValidateRelationEvidence(assertion, valid), ErrRelationEvidenceInvalid) {
		t.Fatal("evidence type must match assertion method")
	}
}

func TestResolveCurrentRelationsFollowsCorrectionAndRetraction(t *testing.T) {
	first := validSecurityRelation("r1", string(RelationConfirmed))
	correction := validSecurityRelation("r2", string(RelationConfirmed))
	correction.Security = &SecurityRelationSubject{Market: "XSHE", Symbol: "000001", Name: "平安银行"}
	correction.Method = RelationManual
	correction.SupersedesRelationID = "r1"
	current, err := ResolveCurrentRelations([]RelationAssertion{first, correction})
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || current[0].ID != "r2" {
		t.Fatalf("current=%+v", current)
	}

	retraction := validSecurityRelation("r3", string(RelationRetracted))
	retraction.Method = RelationManual
	retraction.SupersedesRelationID = "r2"
	current, err = ResolveCurrentRelations([]RelationAssertion{first, correction, retraction})
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 0 {
		t.Fatalf("retracted relation remains current: %+v", current)
	}
}

func TestResolveCurrentRelationsRejectsMultipleSuccessors(t *testing.T) {
	first := validSecurityRelation("r1", string(RelationConfirmed))
	a := validSecurityRelation("r2", string(RelationConfirmed))
	a.Method, a.SupersedesRelationID = RelationManual, "r1"
	b := validSecurityRelation("r3", string(RelationRetracted))
	b.Method, b.SupersedesRelationID = RelationManual, "r1"
	if _, err := ResolveCurrentRelations([]RelationAssertion{first, a, b}); !errors.Is(err, ErrRelationHistoryConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestResolveCurrentRelationsRejectsCrossNewsAndMissingParent(t *testing.T) {
	first := validSecurityRelation("r1", string(RelationConfirmed))
	correction := validSecurityRelation("r2", string(RelationConfirmed))
	correction.Method, correction.SupersedesRelationID = RelationManual, "missing"
	if _, err := ResolveCurrentRelations([]RelationAssertion{first, correction}); !errors.Is(err, ErrRelationHistoryConflict) {
		t.Fatalf("missing parent err=%v", err)
	}
	correction.SupersedesRelationID = "r1"
	correction.NewsID = "news-2"
	if _, err := ResolveCurrentRelations([]RelationAssertion{first, correction}); !errors.Is(err, ErrRelationHistoryConflict) {
		t.Fatalf("cross-news err=%v", err)
	}
}
