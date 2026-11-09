package context

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func validContext() Context {
	cutoff := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	return Context{
		SchemaVersion: "1.0.0", ContextID: "ctx-001", ContextVersion: 1,
		ResearchQuestion: "分析证券 600000.SH 的趋势", Scope: ScopeStock,
		SubjectRefs: []string{"600000.SH"}, SystemAsOf: cutoff,
		SectionRefs: []SectionRef{
			{SectionID: "stock_profile", SectionVersion: "v1", SourceOwner: SourceOwnerData, SourceRefs: []string{"data:snapshot:001"}, EvidenceRefs: []string{}, QualityStatus: QualityValid, Required: true, Included: true, AssembledAt: cutoff, SystemAsOf: cutoff},
			{SectionID: "risk_review", SectionVersion: "v1", SourceOwner: SourceOwnerResearch, SourceRefs: []string{}, EvidenceRefs: []string{}, QualityStatus: QualityUnknown, Required: false, Included: false, AssembledAt: cutoff, SystemAsOf: cutoff},
		},
		QualitySummary: QualitySummary{Status: QualityDegraded, MissingOptionalCount: 1, ReasonCodes: []string{"optional_section_omitted"}},
		IncludedSections: []string{"stock_profile"},
		OmittedSections: []OmittedSection{{SectionID: "risk_review", SourceOwner: SourceOwnerResearch, ReasonCode: OmissionNotRequested, Required: false, SourceRefs: []string{}, SafeMessage: "该研究请求未要求风险分区", Retryable: false}},
		BuilderVersion: "research-context-v1", CreatedAt: cutoff,
	}
}

func TestContextValidateAcceptsValidContext(t *testing.T) {
	if err := validContext().Validate(); err != nil { t.Fatalf("Validate() error = %v", err) }
}

func TestContextValidateRejectsInvalidStructure(t *testing.T) {
	tests := []struct { name string; mutate func(*Context); want string }{
		{"invalid schema version", func(c *Context) { c.SchemaVersion = "v1" }, "schema_version"},
		{"schema version leading zero", func(c *Context) { c.SchemaVersion = "01.0.0" }, "schema_version"},
		{"padded subject ref", func(c *Context) { c.SubjectRefs = []string{" 600000.SH"} }, "whitespace-padded"},
		{"padded section id", func(c *Context) { c.SectionRefs[0].SectionID = " stock_profile" }, "without surrounding whitespace"},
		{"missing id", func(c *Context) { c.ContextID = " " }, "context_id"},
		{"padded context id", func(c *Context) { c.ContextID = " ctx-001" }, "surrounding whitespace"},
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
	if err := value.ValidateSystemAsOf(value.SystemAsOf); err != nil { t.Fatalf("matching cutoff unexpectedly failed: %v", err) }
	if err := value.ValidateSystemAsOf(value.SystemAsOf.Add(time.Second)); err == nil { t.Fatal("mismatched cutoff must fail") }
	if err := value.ValidateSystemAsOf(value.SystemAsOf.In(time.FixedZone("non-UTC", 0))); err == nil {
		t.Fatal("equivalent instant in a non-UTC location must fail")
	}
}

func TestContextJSONRoundTripPreservesRequiredEmptyArrays(t *testing.T) {
	want := validContext()
	encoded, err := json.Marshal(want)
	if err != nil { t.Fatalf("Marshal() error = %v", err) }
	var got Context
	if err := json.Unmarshal(encoded, &got); err != nil { t.Fatalf("Unmarshal() error = %v", err) }
	if err := got.Validate(); err != nil { t.Fatalf("round-tripped Context is invalid: %v", err) }
	if got.SectionRefs[1].SourceRefs == nil || got.SectionRefs[1].EvidenceRefs == nil || got.OmittedSections[0].SourceRefs == nil {
		t.Fatal("required empty arrays must remain non-null after JSON round-trip")
	}
}


func TestContextValidateNextVersion(t *testing.T) {
	previous := validContext()
	next := validContext()
	next.ContextVersion = 2
	next.CreatedAt = next.CreatedAt.Add(time.Minute)
	next.SystemAsOf = next.SystemAsOf.Add(time.Minute)
	for i := range next.SectionRefs {
		next.SectionRefs[i].SystemAsOf = next.SystemAsOf
		next.SectionRefs[i].AssembledAt = next.SystemAsOf
	}
	if err := next.ValidateNextVersion(previous); err != nil {
		t.Fatalf("valid next version unexpectedly failed: %v", err)
	}
}

func TestContextValidateNextVersionRejectsInvalidProgression(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Context)
		want   string
	}{
		{"context id changed", func(c *Context) { c.ContextID = "ctx-002" }, "context_id"},
		{"version skipped", func(c *Context) { c.ContextVersion = 3 }, "increment by exactly one"},
		{"knowledge cutoff moved backwards", func(c *Context) {
			c.SystemAsOf = c.SystemAsOf.Add(-time.Minute)
			for i := range c.SectionRefs {
				c.SectionRefs[i].SystemAsOf = c.SystemAsOf
			}
		}, "system_as_of cannot move backwards"},
		{"created time moved backwards", func(c *Context) { c.CreatedAt = c.CreatedAt.Add(-time.Minute) }, "created_at cannot move backwards"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := validContext()
			next := validContext()
			next.ContextVersion = 2
			next.CreatedAt = next.CreatedAt.Add(time.Minute)
			next.SystemAsOf = next.SystemAsOf.Add(time.Minute)
			for i := range next.SectionRefs {
				next.SectionRefs[i].SystemAsOf = next.SystemAsOf
				next.SectionRefs[i].AssembledAt = next.SystemAsOf
			}
			tt.mutate(&next)
			err := next.ValidateNextVersion(previous)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateNextVersion() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}


func TestContextValidateNextVersionRejectsIntegerOverflow(t *testing.T) {
	previous := validContext()
	previous.ContextVersion = math.MaxInt64
	next := previous
	next.ContextVersion = 1
	if err := next.ValidateNextVersion(previous); err == nil || !strings.Contains(err.Error(), "integer limit") {
		t.Fatalf("ValidateNextVersion() error = %v, want integer limit rejection", err)
	}
}

func TestContextGoldenJSONV1(t *testing.T) {
	want := validContext()
	encoded, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	encoded = append(encoded, '\n')

	golden, err := os.ReadFile("testdata/context_v1.golden.json")
	if err != nil {
		t.Fatalf("read Context v1 golden fixture: %v", err)
	}
	if !bytes.Equal(encoded, golden) {
		t.Fatalf("Context v1 JSON differs from golden fixture; update the fixture only after reviewing the public DTO contract")
	}
}

func TestContextValidateRejectsUnsupportedSchemaVersionAndNonUTCTimes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Context)
		want   string
	}{
		{"unsupported schema version", func(c *Context) { c.SchemaVersion = "1.0.1" }, "supported version"},
		{"non UTC context cutoff", func(c *Context) { c.SystemAsOf = c.SystemAsOf.In(time.FixedZone("UTC+8", 8*60*60)) }, "system_as_of"},
		{"zero-offset non-UTC context cutoff", func(c *Context) { c.SystemAsOf = c.SystemAsOf.In(time.FixedZone("non-UTC", 0)) }, "system_as_of"},
		{"non UTC created time", func(c *Context) { c.CreatedAt = c.CreatedAt.In(time.FixedZone("UTC+8", 8*60*60)) }, "created_at"},
		{"non UTC section assembled time", func(c *Context) { c.SectionRefs[0].AssembledAt = c.SectionRefs[0].AssembledAt.In(time.FixedZone("UTC+8", 8*60*60)) }, "assembled_at"},
		{"non UTC section cutoff", func(c *Context) { c.SectionRefs[0].SystemAsOf = c.SectionRefs[0].SystemAsOf.In(time.FixedZone("UTC+8", 8*60*60)) }, "system_as_of"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := validContext()
			tt.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
