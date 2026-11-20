package repository

import (
	"encoding/json"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

type dailyBarMetadata struct {
	model.DailyBar
	ExactValues *model.DailyBarExactValues `json:"exact_values,omitempty"`
}

func marshalDailyBarMetadata(bar model.DailyBar) ([]byte, error) {
	exactValues := bar.ExactValues
	bar.ExactValues = nil
	return json.Marshal(dailyBarMetadata{DailyBar: bar, ExactValues: exactValues})
}

// unmarshalDailyBarMetadata reads both legacy flat DailyBar metadata and the
// additive exact_values sidecar without changing the public DailyBar JSON shape.
func unmarshalDailyBarMetadata(data []byte, bar *model.DailyBar) error {
	if bar == nil {
		return json.Unmarshal(data, bar)
	}
	if err := json.Unmarshal(data, bar); err != nil {
		return err
	}
	var metadata dailyBarMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return err
	}
	bar.ExactValues = metadata.ExactValues
	return nil
}
