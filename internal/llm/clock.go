package llm

import (
	"time"

	platformtime "example.com/stock-analysis-flow/internal/platform/time"
)

// Clock preserves the LLM application clock port.
type Clock interface {
	Now() time.Time
}

// SystemClock reuses the shared Platform operating-system clock.
type SystemClock = platformtime.SystemClock

var _ Clock = SystemClock{}
