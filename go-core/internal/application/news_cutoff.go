package application

import (
	"fmt"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

// filterNewsByReference prevents evidence published after the analysis reference
// time from entering a report. For explicit point-in-time requests, items without
// a trustworthy publication timestamp are excluded because their historical
// availability cannot be proven. For live requests they remain visible, but are
// explicitly reported as unverified.
func filterNewsByReference(items []domain.NewsItem, reference time.Time, strict bool) ([]domain.NewsItem, []string) {
	if reference.IsZero() || len(items) == 0 {
		return items, nil
	}
	out := make([]domain.NewsItem, 0, len(items))
	future, unverified := 0, 0
	for _, item := range items {
		stamp := strings.TrimSpace(item.PublishedAt)
		if stamp == "" {
			unverified++
			if !strict {
				out = append(out, item)
			}
			continue
		}
		publishedAt, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			unverified++
			if !strict {
				out = append(out, item)
			}
			continue
		}
		if publishedAt.After(reference) {
			future++
			continue
		}
		out = append(out, item)
	}
	warnings := make([]string, 0, 2)
	if future > 0 {
		warnings = append(warnings, fmt.Sprintf("excluded %d news item(s) published after analysis reference time %s", future, reference.UTC().Format(time.RFC3339Nano)))
	}
	if unverified > 0 {
		if strict {
			warnings = append(warnings, fmt.Sprintf("excluded %d news item(s) with missing or invalid publication timestamps from explicit as_of evidence", unverified))
		} else {
			warnings = append(warnings, fmt.Sprintf("%d news item(s) have missing or invalid publication timestamps; retained only as unverified live evidence", unverified))
		}
	}
	return out, warnings
}
