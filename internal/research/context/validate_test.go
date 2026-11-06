package context

import (
	"strings"
	"testing"
	"time"
)

func validContext() Context {
	cutoff := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	return Context{
		SchemaVersion:    "1.0.0",
		ContextID:        "ctx-001",
		ContextVersion:   1,
		ResearchQuestion: "分析证券 600000.SH 的趋势",
		Scope:            ScopeStock,
		SubjectRefs:      []string{"600000.SH"},
		SystemAsOf:       cutoff,
		SectionRefs: []SectionRef{
			{SectionID: "stock_profile", SectionVersion: "v1", SourceOwner: SourceOwnerData, SourceRefs: []string{"data:snapshot:001"}, EvidenceRefs: []string{}, QualityStatus: QualityValid, Required: true, Included: true, AssembledAt: cutoff, SystemAsOf: cutoff},
			{SectionID: "risk_review", SectionVersion: "v1", SourceOwner: SourceOwnerResearch, SourceRefs: []string{}, EvidenceRefs: []string{}, QualityStatus: QualityUnknown, Required: false, Included: false, AssembledAt: cutoff, SystemAsOf: cutoff},
		},
		QualitySummary:   QualitySummary{Status: QualityDegraded, MissingOptionalCount: 1, ReasonCodes: []string{"optional_section_omitted"}},
		IncludedSections: []string{"stock_profile"},
		OmittedSections:  []OmittedSection{{SectionID: "risk_review", SourceOwner: SourceOwnerResearch, ReasonCode: OmissionNotRequested, Required: false, SourceRefs: []string{}, SafeMessage: "该研究请求未要求风险分区", Retryable: false}},
		BuilderVersion:   "research-context-v1",
		CreatedAt:        cutoff,
	}
}

func TestContextValidateAcceptsValidContext(t *testing.T) {
	if err := validContext().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestContextValidateRejectsInvalidStructure(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Context)
		want   string
	}{
		{"invalid schema version", func(c *Context) { c.SchemaVersion = "v1" }, "schema_version"},
		{"missing id", func(c *Context) { c.ContextID = " " }, "context_id"},
		{"zero version", func(c *Context) { c.ContextVersion = 0 }, "context_version"},
		{"missing question", func(c *Context) { c.ResearchQuestion = "" }, "research_question"},
		{"invalid scope", func(c *Context) { c.Scope = "portfolio" }, "scope"},
		{"nil subject refs", func(c *Context) { c.SubjectRefs = nil }, "subject_refs"},
		{"missing cutoff", func(c *Context) { c.SystemAsOf = time.Time{} }, "system_as_of"},
		{"nil section refs", func(c *Context) { c.SectionRefs = nil }, "section_refs"},
		{"duplicate section id", func(c *Context) { c.SectionRefs = append(c.SectionRefs, c.SectionRefs[0]) }, "duplicate section_id"},
		{"section cutoff mismatch", func(c *Context) { c.SectionRefs[0].SystemAsOf = c.SystemAsOf.Add(time.Second) }, "does not match Context"},
		{"nil included list", func(c *Context) { c.IncludedSections = nil }, "non-null arrays"},
		{"incomplete included list", func(c *Context) { c.IncludedSections = []string{} }, "does not match section_refs"},
		{"required section omitted as valid", func(c *Context) {
			c.SectionRefs[0].Included = false
			c.IncludedSections = []string{}
			c.OmittedSections = append(c.OmittedSections, OmittedSection{SectionID: "stock_profile", SourceOwner: SourceOwnerData, ReasonCode: OmissionUnavailable, Required: true, SourceRefs: []string{"data:snapshot:001"}, SafeMessage: "不可用"})
			c.QualitySummary = QualitySummary{Status: QualityValid, MissingRequiredCount: 1, MissingOptionalCount: 1, ReasonCodes: []string{}}
		}, "cannot have valid"},
		{"negative count", func(c *Context) { c.QualitySummary.ConflictCount = -1 }, "cannot be negative"},
		{"unsorted reasons", func(c *Context) { c.QualitySummary.ReasonCodes = []string{"z_reason", "a_reason"} }, "must be sorted"},
		{"omitted reason invalid", func(c *Context) { c.OmittedSections[0].ReasonCode = "unknown_reason" }, "reason_code is invalid"},
		{"omitted safe message missing", func(c *Context) { c.OmittedSections[0].SafeMessage = " " }, "safe_message"},
		{"omitted metadata mismatch", func(c *Context) { c.OmittedSections[0].Required = true }, "metadata does not match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := validContext()
			tt.mutate(&value)
			err := value.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestContextValidateSystemAsOf(t *testing.T) {
	value := validContext()
	if err := value.ValidateSystemAsOf(value.SystemAsOf); err != nil {
		t.Fatalf("matching cutoff unexpectedly failed: %v", err)
	}
	if err := value.ValidateSystemAsOf(value.SystemAsOf.Add(time.Second)); err == nil {
		t.Fatal("mismatched cutoff must fail")
	}
}
