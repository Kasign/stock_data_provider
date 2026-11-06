package app

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func validEvidenceListRequest(now time.Time) ListEvidenceRequest {
	return ListEvidenceRequest{
		CallerRef: "research",
		PurposeCode: EvidencePurposeResearchAnalysis,
		RequestedContentLevel: EvidenceContentSummary,
		SystemAsOf: now.Add(-time.Minute),
		Filters: &EvidenceQueryFilters{
			NewsIDs: []string{"news-1"},
			SourceRefs: []string{"source-1"},
			CanonicalSymbols: []string{"SSE:600000"},
			QualityStatuses: []EvidenceQualityStatus{EvidenceQualityValid, EvidenceQualityDegraded},
		},
		PageSize: 20,
	}
}

func TestListEvidenceRequestValidateAcceptsContractBoundedQuery(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	request := validEvidenceListRequest(now)
	if err := request.Validate(now); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestListEvidenceRequestValidateRejectsMissingRequiredFields(t *testing.T) {
	now := time.Now().UTC()
	base := validEvidenceListRequest(now)
	cases := []struct {
		name string
		mutate func(*ListEvidenceRequest)
	}{
		{"caller", func(r *ListEvidenceRequest) { r.CallerRef = "" }},
		{"purpose", func(r *ListEvidenceRequest) { r.PurposeCode = "internal_business" }},
		{"content level", func(r *ListEvidenceRequest) { r.RequestedContentLevel = "full_text" }},
		{"system as of", func(r *ListEvidenceRequest) { r.SystemAsOf = time.Time{} }},
		{"filters", func(r *ListEvidenceRequest) { r.Filters = nil }},
		{"page size zero", func(r *ListEvidenceRequest) { r.PageSize = 0 }},
		{"page size too large", func(r *ListEvidenceRequest) { r.PageSize = 101 }},
		{"explicit empty cursor", func(r *ListEvidenceRequest) { empty := ""; r.Cursor = &empty }},
		{"cursor too large", func(r *ListEvidenceRequest) { cursor := string(make([]byte, 4097)); r.Cursor = &cursor }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := base
			request.Filters = cloneEvidenceFilters(base.Filters)
			testCase.mutate(&request)
			if err := request.Validate(now); !errors.Is(err, ErrInvalidEvidenceQuery) {
				t.Fatalf("Validate() error = %v, want ErrInvalidEvidenceQuery", err)
			}
		})
	}
}

func TestListEvidenceRequestValidateRejectsOutOfRangeAndDuplicateFilters(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name string
		mutate func(*EvidenceQueryFilters)
	}{
		{"duplicate news id", func(f *EvidenceQueryFilters) { f.NewsIDs = []string{"news-1", "news-1"} }},
		{"too many source refs", func(f *EvidenceQueryFilters) {
			f.SourceRefs = make([]string, 51)
			for i := range f.SourceRefs { f.SourceRefs[i] = "source-" + string(rune('a'+i%26)) + string(rune('A'+i/26)) }
		}},
		{"invalid quality status", func(f *EvidenceQueryFilters) { f.QualityStatuses = []EvidenceQualityStatus{"fresh"} }},
		{"duplicate quality status", func(f *EvidenceQueryFilters) { f.QualityStatuses = []EvidenceQualityStatus{EvidenceQualityValid, EvidenceQualityValid} }},
		{"blank source ref", func(f *EvidenceQueryFilters) { f.SourceRefs = []string{" "} }},
		{"control character", func(f *EvidenceQueryFilters) { f.NewsIDs = []string{"news\n1"} }},
		{"reversed published range", func(f *EvidenceQueryFilters) {
			from := now
			to := now.Add(-time.Hour)
			f.PublishedAtFrom, f.PublishedAtTo = &from, &to
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := validEvidenceListRequest(now)
			testCase.mutate(request.Filters)
			if err := request.Validate(now); !errors.Is(err, ErrInvalidEvidenceQuery) {
				t.Fatalf("Validate() error = %v, want ErrInvalidEvidenceQuery", err)
			}
		})
	}
}

func TestListEvidenceRequestKeepsBusinessTimeIndependentAndAllowsFutureFilter(t *testing.T) {
	now := time.Now().UTC()
	request := validEvidenceListRequest(now)
	from := now.Add(time.Hour)
	to := now.Add(2 * time.Hour)
	request.BusinessTimeFrom, request.BusinessTimeTo = &from, &to
	if err := request.Validate(now); err != nil {
		t.Fatalf("future business-time filter should be valid: %v", err)
	}
}

func TestEvidenceQueryRejectsFutureOrNonUTCSystemAsOf(t *testing.T) {
	now := time.Now().UTC()
	request := validEvidenceListRequest(now)
	request.SystemAsOf = now.Add(time.Second)
	if err := request.Validate(now); !errors.Is(err, ErrInvalidEvidenceQuery) {
		t.Fatalf("future system_as_of error = %v", err)
	}
	request.SystemAsOf = now.In(time.FixedZone("UTC+8", 8*60*60))
	if err := request.Validate(now); !errors.Is(err, ErrInvalidEvidenceQuery) {
		t.Fatalf("non-UTC system_as_of error = %v", err)
	}
}

func TestGetEvidenceRequestRequiresNewsAndKnowledgeTime(t *testing.T) {
	now := time.Now().UTC()
	request := GetEvidenceRequest{
		CallerRef: "research",
		PurposeCode: EvidencePurposeRiskReview,
		RequestedContentLevel: EvidenceContentMetadata,
		SystemAsOf: now,
		NewsID: "news-1",
		VersionID: "version-1",
	}
	if err := request.Validate(now); err != nil {
		t.Fatalf("valid GetEvidence request: %v", err)
	}
	request.SystemAsOf = time.Time{}
	if err := request.Validate(now); !errors.Is(err, ErrInvalidEvidenceQuery) {
		t.Fatalf("missing system_as_of error = %v", err)
	}
}

func TestResearchEvidencePageInitializesNonNullArrays(t *testing.T) {
	page := NewResearchEvidencePage(time.Now())
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if string(decoded["items"]) != "[]" || string(decoded["diagnostics"]) != "[]" {
		t.Fatalf("empty arrays must serialize as []: %s", encoded)
	}
}

func cloneEvidenceFilters(filters *EvidenceQueryFilters) *EvidenceQueryFilters {
	if filters == nil {
		return nil
	}
	copy := *filters
	copy.NewsIDs = append([]string(nil), filters.NewsIDs...)
	copy.SourceRefs = append([]string(nil), filters.SourceRefs...)
	copy.CanonicalSymbols = append([]string(nil), filters.CanonicalSymbols...)
	copy.QualityStatuses = append([]EvidenceQualityStatus(nil), filters.QualityStatuses...)
	return &copy
}
