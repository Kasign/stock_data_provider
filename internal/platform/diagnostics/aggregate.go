package diagnostics

import "example.com/stock-analysis-flow/internal/platform/fault"

type Counts struct {
	Healthy     int `json:"healthy"`
	Degraded    int `json:"degraded"`
	Unavailable int `json:"unavailable"`
	Skipped     int `json:"skipped"`
	Failed      int `json:"failed"`
}

type Summary struct {
	Status Status `json:"status"`
	Counts Counts `json:"counts"`
}

type Result struct {
	CheckID     string       `json:"check_id"`
	Name        string       `json:"name,omitempty"`
	Required    bool         `json:"required"`
	Observation Observation  `json:"observation"`
	Fault       *fault.Fault `json:"fault,omitempty"`
}

type Report struct {
	Summary Summary  `json:"summary"`
	Results []Result `json:"results"`
}

func aggregate(results []Result) Summary {
	if len(results) == 0 {
		return Summary{Status: StatusSkipped}
	}

	var counts Counts
	requiredFailed := false
	requiredUnavailable := false
	hasDegraded := false
	hasOptionalFailure := false
	allSkipped := true

	for _, result := range results {
		switch result.Observation.Status {
		case StatusHealthy:
			counts.Healthy++
			allSkipped = false
		case StatusDegraded:
			counts.Degraded++
			hasDegraded = true
			allSkipped = false
		case StatusUnavailable:
			counts.Unavailable++
			allSkipped = false
			if result.Required {
				requiredUnavailable = true
			} else {
				hasOptionalFailure = true
			}
		case StatusSkipped:
			counts.Skipped++
		case StatusFailed:
			counts.Failed++
			allSkipped = false
			if result.Required {
				requiredFailed = true
			} else {
				hasOptionalFailure = true
			}
		}
	}

	status := StatusHealthy
	switch {
	case requiredFailed:
		status = StatusFailed
	case requiredUnavailable:
		status = StatusUnavailable
	case hasDegraded || hasOptionalFailure:
		status = StatusDegraded
	case allSkipped:
		status = StatusSkipped
	}
	return Summary{Status: status, Counts: counts}
}
