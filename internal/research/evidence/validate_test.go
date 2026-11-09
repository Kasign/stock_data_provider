package evidence

import (
    "encoding/json"
    "math"
    "strings"
    "testing"
    "time"
)

const goldenFingerprint = "56ddf6b7f4c1964ebaa0489dc26e3f34a8a37950080d9e31dd687bddcd7af3cd"

func goldenEvidence(t *testing.T) Evidence {
    t.Helper()
    publishedAt := mustTime(t, "2026-11-05T08:00:00Z")
    availableAt := mustTime(t, "2026-11-05T08:00:00Z")
    observedAt := mustTime(t, "2026-11-05T08:01:00Z")
    systemAsOf := mustTime(t, "2026-11-06T05:00:00Z")
    sourceVersion := "v1"
    sourceLocator := "news://news_001/version/v1"
    return Evidence{
        EvidenceID: "evidence:research:0001",
        EvidenceType: TypeReportedFact,
        SourceOwner: SourceNews,
        SourceRef: "news_evidence_version:news_001:v1",
        SourceVersion: &sourceVersion,
        SourceLocator: &sourceLocator,
        DerivedFromRefs: []string{},
        ClaimRef: nil,
        SubjectRefs: []string{"600000.SH"},
        PublishedAt: &publishedAt,
        AvailableAt: &availableAt,
        SystemObservedAt: observedAt,
        ValidTimeStart: nil,
        ValidTimeEnd: nil,
        SystemAsOf: systemAsOf,
        Stance: StanceSupports,
        Confidence: nil,
        ConfidenceMethod: nil,
        ConfidenceMethodVersion: nil,
        QualityStatus: QualityValid,
        ContentLevel: ContentSnippet,
        AuthorizationRef: "auth_news_001",
        Limitations: []string{"source_attribution_required", "snippet_only"},
        ContentFingerprint: goldenFingerprint,
    }
}

func mustTime(t *testing.T, value string) time.Time {
    t.Helper()
    parsed, err := time.Parse(time.RFC3339Nano, value)
    if err != nil {
        t.Fatalf("parse time %q: %v", value, err)
    }
    return parsed
}

func setFingerprint(t *testing.T, value *Evidence) {
    t.Helper()
    fingerprint, err := value.ComputeContentFingerprint()
    if err != nil {
        t.Fatalf("ComputeContentFingerprint() error = %v", err)
    }
    value.ContentFingerprint = fingerprint
}

func TestEvidenceFingerprintMatchesRSCCTR001Golden(t *testing.T) {
    value := goldenEvidence(t)
    fingerprint, err := value.ComputeContentFingerprint()
    if err != nil {
        t.Fatalf("ComputeContentFingerprint() error = %v", err)
    }
    if fingerprint != goldenFingerprint {
        t.Fatalf("fingerprint = %s, want %s", fingerprint, goldenFingerprint)
    }
    if err := value.Validate(); err != nil {
        t.Fatalf("Validate() error = %v", err)
    }
}

func TestEvidenceValidateRejectsInvalidContractValues(t *testing.T) {
    tests := []struct {
        name string
        mutate func(*Evidence)
        want string
    }{
        {"invalid evidence type", func(e *Evidence) { e.EvidenceType = "assertion" }, "evidence_type"},
        {"invalid source owner", func(e *Evidence) { e.SourceOwner = "platform" }, "source_owner"},
        {"missing source ref", func(e *Evidence) { e.SourceRef = " " }, "source_ref"},
        {"padded source ref", func(e *Evidence) { e.SourceRef = " news_ref" }, "surrounding whitespace"},
        {"nil derived refs", func(e *Evidence) { e.DerivedFromRefs = nil }, "non-null arrays"},
        {"duplicate subject refs", func(e *Evidence) { e.SubjectRefs = []string{"600000.SH", "600000.SH"} }, "duplicate"},
        {"inference without lineage", func(e *Evidence) { e.EvidenceType = TypeInference }, "requires derived_from_refs"},
        {"missing authorization", func(e *Evidence) { e.AuthorizationRef = "" }, "authorization_ref"},
        {"invalid confidence", func(e *Evidence) { v := 1.1; e.Confidence = &v }, "within [0,1]"},
        {"confidence without method", func(e *Evidence) { v := 0.8; e.Confidence = &v }, "confidence_method is required"},
        {"confidence without method version", func(e *Evidence) { v := 0.8; m := "method"; e.Confidence = &v; e.ConfidenceMethod = &m }, "confidence_method_version is required"},
        {"unpaired valid time", func(e *Evidence) { v := mustTime(t, "2026-11-01T00:00:00Z"); e.ValidTimeStart = &v }, "both be null or both be set"},
        {"reversed valid time", func(e *Evidence) { start := mustTime(t, "2026-11-02T00:00:00Z"); end := mustTime(t, "2026-11-01T00:00:00Z"); e.ValidTimeStart = &start; e.ValidTimeEnd = &end }, "must not be after"},
        {"non-UTC observed time", func(e *Evidence) { e.SystemObservedAt = time.Date(2026, 11, 5, 16, 1, 0, 0, time.FixedZone("CST", 8*60*60)) }, "must be UTC"},
        {"system cutoff before observation", func(e *Evidence) { e.SystemAsOf = e.SystemObservedAt.Add(-time.Second); setFingerprint(t, e) }, "must not be earlier"},
        {"uppercase fingerprint", func(e *Evidence) { e.ContentFingerprint = strings.ToUpper(e.ContentFingerprint) }, "lowercase SHA-256"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            value := goldenEvidence(t)
            tt.mutate(&value)
            if err := value.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
                t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
            }
        })
    }
}

func TestEvidenceStrictPITRequiresAvailableAndObservedBeforeCutoff(t *testing.T) {
    value := goldenEvidence(t)
    if err := value.ValidateForStrictPIT(value.SystemAsOf); err != nil {
        t.Fatalf("strict PIT validation unexpectedly failed: %v", err)
    }

    lateAvailable := goldenEvidence(t)
    available := lateAvailable.SystemAsOf.Add(time.Second)
    lateAvailable.AvailableAt = &available
    setFingerprint(t, &lateAvailable)
    if err := lateAvailable.ValidateForStrictPIT(lateAvailable.SystemAsOf); err == nil || !strings.Contains(err.Error(), "available_at is later") {
        t.Fatalf("late available_at should be rejected, got %v", err)
    }

    withoutAvailable := goldenEvidence(t)
    withoutAvailable.AvailableAt = nil
    setFingerprint(t, &withoutAvailable)
    if err := withoutAvailable.ValidateForStrictPIT(withoutAvailable.SystemAsOf); err == nil || !strings.Contains(err.Error(), "available_at is unknown") {
        t.Fatalf("unknown available_at should be rejected, got %v", err)
    }

    lateObservation := goldenEvidence(t)
    lateObservation.SystemObservedAt = lateObservation.SystemAsOf.Add(time.Second)
    setFingerprint(t, &lateObservation)
    if err := lateObservation.ValidateForStrictPIT(lateObservation.SystemAsOf); err == nil || !strings.Contains(err.Error(), "system_observed_at") {
        t.Fatalf("late observation should be rejected, got %v", err)
    }

    lateEvidenceCutoff := goldenEvidence(t)
    if err := lateEvidenceCutoff.ValidateForStrictPIT(lateEvidenceCutoff.SystemAsOf.Add(-time.Second)); err == nil || !strings.Contains(err.Error(), "later than Run") {
        t.Fatalf("later evidence cutoff should be rejected, got %v", err)
    }
}

func TestEvidenceJSONRoundTripKeepsNullableAndRequiredFields(t *testing.T) {
    want := goldenEvidence(t)
    encoded, err := json.Marshal(want)
    if err != nil {
        t.Fatalf("Marshal() error = %v", err)
    }
    var fields map[string]json.RawMessage
    if err := json.Unmarshal(encoded, &fields); err != nil {
        t.Fatalf("Unmarshal raw fields: %v", err)
    }
    for _, name := range []string{"source_version", "source_locator", "claim_ref", "published_at", "available_at", "valid_time_start", "valid_time_end", "confidence", "confidence_method", "confidence_method_version"} {
        if string(fields[name]) == "" {
            t.Fatalf("required nullable field %q was omitted", name)
        }
    }
    var got Evidence
    if err := json.Unmarshal(encoded, &got); err != nil {
        t.Fatalf("Unmarshal() error = %v", err)
    }
    if err := got.Validate(); err != nil {
        t.Fatalf("round-tripped Evidence is invalid: %v", err)
    }
}

func TestFingerprintExcludesIdentityAndAuthorizationButTracksSemantics(t *testing.T) {
    value := goldenEvidence(t)
    baseline, err := value.ComputeContentFingerprint()
    if err != nil {
        t.Fatal(err)
    }
    value.EvidenceID = "evidence:research:0002"
    value.AuthorizationRef = "auth_rotated"
    unchanged, err := value.ComputeContentFingerprint()
    if err != nil {
        t.Fatal(err)
    }
    if unchanged != baseline {
        t.Fatal("identity and authorization references must not affect content fingerprint")
    }
    value.Stance = StanceContradicts
    changed, err := value.ComputeContentFingerprint()
    if err != nil {
        t.Fatal(err)
    }
    if changed == baseline {
        t.Fatal("semantic changes must change content fingerprint")
    }
}

func TestCanonicalNumberUsesECMAScriptThresholds(t *testing.T) {
    cases := map[float64]string{
        0: "0",
        1e-6: "0.000001",
        1e-7: "1e-7",
        1e6: "1000000",
        1e21: "1e+21",
        0.125: "0.125",
    }
    for input, expected := range cases {
        if got := canonicalNumber(input); got != expected {
            t.Errorf("canonicalNumber(%g) = %q, want %q", input, got, expected)
        }
    }
    negativeZero := math.Copysign(0, -1)
    if got := canonicalNumber(negativeZero); got != "0" {
        t.Errorf("canonicalNumber(-0) = %q, want 0", got)
    }
}

func TestCanonicalObjectSupportsNestedJCSValues(t *testing.T) {
	value := map[string]any{
		"evaluated_policy_versions": []any{
			map[string]any{
				"policy_type": "research_evidence_access",
				"policy_ref": "research-evidence-access",
				"policy_version": "1.0.0",
			},
		},
		"allowed": true,
		"conditions": []string{},
	}
	canonical, err := canonicalObject(value)
	if err != nil {
		t.Fatalf("canonicalObject() error = %v", err)
	}
	expected := `{"allowed":true,"conditions":[],"evaluated_policy_versions":[{"policy_ref":"research-evidence-access","policy_type":"research_evidence_access","policy_version":"1.0.0"}]}`
	if canonical != expected {
		t.Fatalf("canonical JSON = %s, want %s", canonical, expected)
	}
}

func TestCanonicalObjectUsesUTF16PropertyOrdering(t *testing.T) {
	canonical, err := canonicalObject(map[string]any{
		"𐀀": "astral",
		"": "bmp",
	})
	if err != nil {
		t.Fatalf("canonicalObject() error = %v", err)
	}
	expected := `{"𐀀":"astral","":"bmp"}`
	if canonical != expected {
		t.Fatalf("canonical JSON = %q, want %q", canonical, expected)
	}
}

func TestCanonicalValueRejectsNonFiniteNumbers(t *testing.T) {
	if _, err := canonicalValue(math.NaN()); err == nil {
		t.Fatal("NaN must be rejected by JCS")
	}
	if _, err := canonicalValue(math.Inf(1)); err == nil {
		t.Fatal("Infinity must be rejected by JCS")
	}
}

func TestEvidenceValidateRejectsZeroOffsetNonUTCLocations(t *testing.T) {
	fixedZero := time.FixedZone("non-UTC", 0)
	nonUTC := time.Date(2026, 11, 5, 8, 0, 0, 0, fixedZero)
	tests := []struct {
		name string
		mutate func(*Evidence)
	}{
		{"system_observed_at", func(e *Evidence) { e.SystemObservedAt = nonUTC }},
		{"system_as_of", func(e *Evidence) { e.SystemAsOf = nonUTC }},
		{"published_at", func(e *Evidence) { e.PublishedAt = &nonUTC }},
		{"available_at", func(e *Evidence) { e.AvailableAt = &nonUTC }},
		{"valid_time_start", func(e *Evidence) {
			end := nonUTC.Add(time.Hour).UTC()
			e.ValidTimeStart = &nonUTC
			e.ValidTimeEnd = &end
		}},
		{"valid_time_end", func(e *Evidence) {
			start := nonUTC.Add(-time.Hour).UTC()
			e.ValidTimeStart = &start
			e.ValidTimeEnd = &nonUTC
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := goldenEvidence(t)
			tt.mutate(&value)
			setFingerprint(t, &value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "must be UTC") {
				t.Fatalf("Validate() error = %v, want strict UTC Location rejection", err)
			}
		})
	}
}

func TestEvidenceJSONSerializationPreservesUTCNanosecondsAndExplicitNulls(t *testing.T) {
	value := goldenEvidence(t)
	value.SystemObservedAt = time.Date(2026, 11, 5, 8, 1, 2, 123456789, time.UTC)
	value.SystemAsOf = time.Date(2026, 11, 6, 5, 0, 3, 987654321, time.UTC)
	value.SubjectRefs = []string{}
	value.Limitations = []string{}
	value.SourceVersion = nil
	value.SourceLocator = nil
	value.ClaimRef = nil
	value.PublishedAt = nil
	value.AvailableAt = nil
	value.ValidTimeStart = nil
	value.ValidTimeEnd = nil
	value.Confidence = nil
	value.ConfidenceMethod = nil
	value.ConfidenceMethodVersion = nil
	setFingerprint(t, &value)

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("Unmarshal raw fields: %v", err)
	}
	want := map[string]string{
		"system_observed_at":        "\"2026-11-05T08:01:02.123456789Z\"",
		"system_as_of":              "\"2026-11-06T05:00:03.987654321Z\"",
		"subject_refs":              "[]",
		"limitations":               "[]",
		"source_version":            "null",
		"source_locator":            "null",
		"claim_ref":                 "null",
		"published_at":              "null",
		"available_at":              "null",
		"valid_time_start":          "null",
		"valid_time_end":            "null",
		"confidence":                "null",
		"confidence_method":         "null",
		"confidence_method_version": "null",
	}
	for name, expected := range want {
		if got := string(fields[name]); got != expected {
			t.Errorf("%s = %s, want %s", name, got, expected)
		}
	}
}
