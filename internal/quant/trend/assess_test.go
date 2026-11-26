package trend

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/metric"
)

func assessSystemAsOf() time.Time {
	return time.Date(2026, 11, 26, 6, 0, 0, 0, time.UTC)
}

func assessEvidence(t *testing.T, values [4]string, statuses [4]metric.Status, samples [4]int) []metric.Metric {
	t.Helper()
	codes := [4]string{EvidenceMADistanceCode, EvidenceMomentum5Code, EvidenceMomentum20Code, EvidenceMomentum60Code}
	periods := [4]string{"20d", "5d", "20d", "60d"}
	evidence := make([]metric.Metric, 4)
	for index := range codes {
		value := json.RawMessage("null")
		if statuses[index] == metric.StatusAvailable {
			value = json.RawMessage(`"` + values[index] + `"`)
		}
		sampleCount := samples[index]
		evidence[index] = metric.Metric{
			MetricCode:         codes[index],
			Value:              value,
			Unit:               "ratio",
			Period:             periods[index],
			SystemAsOf:         assessSystemAsOf().Format(time.RFC3339Nano),
			QualityStatus:      metric.QualityValid,
			MetricStatus:       statuses[index],
			CalculationVersion: "1.0.0",
			SampleCount:        &sampleCount,
			LowerBound:         json.RawMessage("null"),
			UpperBound:         json.RawMessage("null"),
		}
	}
	return evidence
}

func TestDirectionOfUsesPureSignsWithoutThresholds(t *testing.T) {
	cases := []struct {
		name     string
		ma       string
		momentum string
		want     Direction
	}{
		{"both positive", "0.000000000001", "0.000000000001", DirectionBullish},
		{"both negative", "-0.000000000001", "-0.000000000001", DirectionBearish},
		{"sign conflict", "0.05", "-0.05", DirectionSideways},
		{"zero is not positive", "0", "0.05", DirectionSideways},
		{"both zero", "0", "0", DirectionSideways},
		{"negative zero form", "-0", "-0.05", DirectionSideways},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := DirectionOf(testCase.ma, testCase.momentum)
			if err != nil {
				t.Fatalf("DirectionOf() error = %v", err)
			}
			if got != testCase.want {
				t.Fatalf("DirectionOf(%s, %s) = %s, want %s", testCase.ma, testCase.momentum, got, testCase.want)
			}
		})
	}
	if _, err := DirectionOf("1e5", "0.1"); !errors.Is(err, ErrInvalidAssessmentInput) {
		t.Fatalf("DirectionOf() accepted a non-canonical decimal: %v", err)
	}
}

func TestAssessProducesDirectionPhaseStrengthAndQualityByClosedTable(t *testing.T) {
	allAvailable := [4]metric.Status{metric.StatusAvailable, metric.StatusAvailable, metric.StatusAvailable, metric.StatusAvailable}
	cases := []struct {
		name          string
		values        [4]string
		statuses      [4]metric.Status
		samples       [4]int
		wantDirection Direction
		wantQuality   QualityStatus
	}{
		{
			"all evidence available", [4]string{"0.05", "0.01", "0.04", "0.09"}, allAvailable, [4]int{20, 6, 21, 61},
			DirectionBullish, QualityValid,
		},
		{
			"sign conflict stays valid", [4]string{"-0.05", "0.01", "0.04", "0.09"}, allAvailable, [4]int{20, 6, 21, 61},
			DirectionSideways, QualityValid,
		},
		{
			"missing long period is degraded", [4]string{"0.05", "0.01", "0.04", ""},
			[4]metric.Status{metric.StatusAvailable, metric.StatusAvailable, metric.StatusAvailable, metric.StatusInsufficientData},
			[4]int{20, 6, 21, 30}, DirectionBullish, QualityDegraded,
		},
		{
			"missing direction evidence is unknown",
			[4]string{"", "", "", ""},
			[4]metric.Status{metric.StatusInsufficientData, metric.StatusAvailable, metric.StatusInsufficientData, metric.StatusInsufficientData},
			[4]int{19, 6, 20, 30}, DirectionUnknown, QualityUnknown,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			evidence := assessEvidence(t, testCase.values, testCase.statuses, testCase.samples)
			result, err := Assess(AssessmentInput{SystemAsOf: assessSystemAsOf(), Evidence: evidence})
			if err != nil {
				t.Fatalf("Assess() error = %v", err)
			}
			if result.Direction != testCase.wantDirection || result.QualityStatus != testCase.wantQuality {
				t.Fatalf("Assess() direction=%s quality=%s, want %s/%s", result.Direction, result.QualityStatus, testCase.wantDirection, testCase.wantQuality)
			}
			if result.Phase != PhaseUnknown || result.Strength != nil || result.Period != CloseTrendPeriod ||
				result.TrendCode != CloseTrendCode || result.AlgorithmVersion != CloseTrendAlgorithmVersion {
				t.Fatalf("Assess() leaked undecided semantics into D3/D4/D5: %+v", result)
			}
			if len(result.ConflictRefs) != 0 || len(result.EvidenceMetrics) != 4 {
				t.Fatalf("Assess() must keep conflict_refs empty and the fixed four evidence metrics: %+v", result)
			}
		})
	}
}

func TestAssessRejectsNonCanonicalEvidence(t *testing.T) {
	allAvailable := [4]metric.Status{metric.StatusAvailable, metric.StatusAvailable, metric.StatusAvailable, metric.StatusAvailable}
	baseline := func() []metric.Metric {
		return assessEvidence(t, [4]string{"0.05", "0.01", "0.04", "0.09"}, allAvailable, [4]int{20, 6, 21, 61})
	}
	cases := []struct {
		name   string
		mutate func([]metric.Metric) []metric.Metric
	}{
		{"wrong evidence order", func(e []metric.Metric) []metric.Metric { e[0], e[1] = e[1], e[0]; return e }},
		{"missing evidence", func(e []metric.Metric) []metric.Metric { return e[:3] }},
		{"period mismatch", func(e []metric.Metric) []metric.Metric { e[2].Period = "60d"; return e }},
		{"unit mismatch", func(e []metric.Metric) []metric.Metric { e[0].Unit = "percent"; return e }},
		{"quality degraded upstream", func(e []metric.Metric) []metric.Metric {
			e[0].QualityStatus = metric.QualityDegraded
			return e
		}},
		{"available with too few samples", func(e []metric.Metric) []metric.Metric {
			short := 19
			e[0].SampleCount = &short
			return e
		}},
		{"insufficient with a value", func(e []metric.Metric) []metric.Metric {
			short := 30
			e[3].MetricStatus = metric.StatusInsufficientData
			e[3].Value = json.RawMessage(`"0.09"`)
			e[3].SampleCount = &short
			return e
		}},
		{"foreign system as of", func(e []metric.Metric) []metric.Metric {
			e[1].SystemAsOf = "2026-11-26T07:00:00Z"
			return e
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			evidence := testCase.mutate(baseline())
			if _, err := Assess(AssessmentInput{SystemAsOf: assessSystemAsOf(), Evidence: evidence}); !errors.Is(err, ErrInvalidAssessmentInput) {
				t.Fatalf("Assess() error = %v, want ErrInvalidAssessmentInput", err)
			}
		})
	}
	if _, err := Assess(AssessmentInput{SystemAsOf: assessSystemAsOf().In(time.FixedZone("CST", 8*3600)), Evidence: baseline()}); !errors.Is(err, ErrInvalidAssessmentInput) {
		t.Fatalf("Assess() accepted a non-UTC SystemAsOf: %v", err)
	}
}
