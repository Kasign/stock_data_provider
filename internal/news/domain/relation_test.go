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

func TestValidateRelationAssertionRejectsNonCanonicalIdentifiers(t *testing.T) {
	for name, mutate := range map[string]func(*RelationAssertion){
		"padded id": func(v *RelationAssertion) { v.ID = " r1 " },
		"padded news id": func(v *RelationAssertion) { v.NewsID = " news-1 " },
		"padded parent id": func(v *RelationAssertion) { v.SupersedesRelationID = " r0 " },
	} {
		t.Run(name, func(t *testing.T) {
			assertion := validSecurityRelation("r1", string(RelationConfirmed))
			mutate(&assertion)
			if !errors.Is(ValidateRelationAssertion(assertion), ErrRelationInvalid) {
				t.Fatal("non-canonical relation identifiers must be rejected")
			}
		})
	}
}

func TestValidateRelationEvidenceRejectsNonCanonicalReferences(t *testing.T) {
	assertion := validSecurityRelation("r1", string(RelationConfirmed))
	base := RelationEvidence{
		RelationID: "r1", Type: RelationEvidenceSourceField,
		ObservationID: "observation-1", FieldPath: "related_securities",
	}
	cases := map[string]func(*RelationEvidence){
		"padded relation id": func(v *RelationEvidence) { v.RelationID = " r1 " },
		"padded observation id": func(v *RelationEvidence) { v.ObservationID = " observation-1 " },
		"padded source version id": func(v *RelationEvidence) { v.SourceVersionID = " version-1 " },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			evidence := base
			mutate(&evidence)
			if !errors.Is(ValidateRelationEvidence(assertion, evidence), ErrRelationEvidenceInvalid) {
				t.Fatal("non-canonical evidence references must be rejected")
			}
		})
	}
}

func TestValidateRelationAssertionRejectsNonCanonicalSubjectIdentifiers(t *testing.T) {
	cases := map[string]func(*RelationAssertion){
		"padded market": func(v *RelationAssertion) { v.Security.Market = " XSHG " },
		"padded symbol": func(v *RelationAssertion) { v.Security.Symbol = " 600000 " },
		"padded classification system": func(v *RelationAssertion) {
			v.SubjectType = RelationSubjectIndustry
			v.Security = nil
			v.Industry = &IndustryRelationSubject{
				ClassificationSystem: " CSI ", ClassificationVersion: "2026",
				IndustryCode: "I01",
			}
		},
		"padded classification version": func(v *RelationAssertion) {
			v.SubjectType = RelationSubjectIndustry
			v.Security = nil
			v.Industry = &IndustryRelationSubject{
				ClassificationSystem: "CSI", ClassificationVersion: " 2026 ",
				IndustryCode: "I01",
			}
		},
		"padded industry code": func(v *RelationAssertion) {
			v.SubjectType = RelationSubjectIndustry
			v.Security = nil
			v.Industry = &IndustryRelationSubject{
				ClassificationSystem: "CSI", ClassificationVersion: "2026",
				IndustryCode: " I01 ",
			}
		},
		"padded industry name": func(v *RelationAssertion) {
			v.SubjectType = RelationSubjectIndustry
			v.Security = nil
			v.Industry = &IndustryRelationSubject{
				ClassificationSystem: "CSI", ClassificationVersion: "2026",
				IndustryName: " 软件服务 ",
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			assertion := validSecurityRelation("r1", string(RelationConfirmed))
			mutate(&assertion)
			if !errors.Is(ValidateRelationAssertion(assertion), ErrRelationInvalid) {
				t.Fatal("non-canonical subject identifiers must be rejected")
			}
		})
	}
}

func TestValidateRelationEvidenceRejectsNonCanonicalMetadata(t *testing.T) {
	assertion := validSecurityRelation("r1", string(RelationConfirmed))
	base := RelationEvidence{
		RelationID: "r1", Type: RelationEvidenceSourceField,
		ObservationID: "observation-1", FieldPath: "related_securities",
	}
	cases := map[string]func(*RelationEvidence){
		"padded field path": func(v *RelationEvidence) { v.FieldPath = " related_securities " },
		"padded rule version": func(v *RelationEvidence) { v.RuleVersion = " v1 " },
		"padded model version": func(v *RelationEvidence) { v.ModelVersion = " model-v1 " },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			evidence := base
			mutate(&evidence)
			if !errors.Is(ValidateRelationEvidence(assertion, evidence), ErrRelationEvidenceInvalid) {
				t.Fatal("non-canonical evidence metadata must be rejected")
			}
		})
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

func TestResolveCurrentRelationsAllowsOnlyOneConfirmedPrimarySubjectPerNews(t *testing.T) {
	security := validSecurityRelation("r-security", string(RelationConfirmed))
	security.Kind = RelationPrimarySubject
	industry := validSecurityRelation("r-industry", string(RelationConfirmed))
	industry.Kind = RelationPrimarySubject
	industry.SubjectType = RelationSubjectIndustry
	industry.Security = nil
	industry.Industry = &IndustryRelationSubject{
		ClassificationSystem: "CSI", ClassificationVersion: "2026",
		IndustryCode: "I01", IndustryName: "软件服务",
	}
	if _, err := ResolveCurrentRelations([]RelationAssertion{security, industry}); !errors.Is(err, ErrRelationHistoryConflict) {
		t.Fatalf("multiple confirmed primary subjects must conflict: %v", err)
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
