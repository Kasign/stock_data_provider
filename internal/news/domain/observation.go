package domain

import "time"

// ObservationStatus 描述一次候选发现的身份处理结果。
type ObservationStatus string

const (
	ObservationUnidentified ObservationStatus = "unidentified"
	ObservationNew          ObservationStatus = "new"
	ObservationMatched      ObservationStatus = "matched"
	ObservationConflict     ObservationStatus = "conflict"
)

// Observation 是不可丢失的发现证据；即使无法建立新闻主体，也应保留候选和原因。
type Observation struct {
	ID             string
	IdempotencyKey string
	SourceID       string
	ProviderCode   string
	QueryDimension string
	Candidate      Candidate
	ObservedAt     time.Time
	IdentityStatus ObservationStatus
	NewsID         string
	ReasonCode     string
}
