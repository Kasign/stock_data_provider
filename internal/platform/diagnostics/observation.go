package diagnostics

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

type Status string

const (
	StatusHealthy     Status = "healthy"
	StatusDegraded    Status = "degraded"
	StatusUnavailable Status = "unavailable"
	StatusSkipped     Status = "skipped"
	StatusFailed      Status = "failed"
)

var validStatuses = map[Status]struct{}{
	StatusHealthy: {}, StatusDegraded: {}, StatusUnavailable: {},
	StatusSkipped: {}, StatusFailed: {},
}

// Observation 是单项检查的标准化、领域无关结果。
type Observation struct {
	Status      Status            `json:"status"`
	ReasonCode  string            `json:"reason_code,omitempty"`
	SafeMessage string            `json:"safe_message"`
	CheckedAt   time.Time         `json:"checked_at"`
	Duration    time.Duration     `json:"duration"`
	EvidenceRef string            `json:"evidence_ref,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// Validate 检查字段结构和大小限制；领域 Adapter 仍需负责属性白名单和敏感信息过滤。
func (o Observation) Validate() error {
	if _, ok := validStatuses[o.Status]; !ok {
		return fmt.Errorf("invalid observation status")
	}
	if strings.TrimSpace(o.SafeMessage) == "" {
		return errors.New("safe_message is required")
	}
	if hasControlCharacters(o.SafeMessage) || hasControlCharacters(o.EvidenceRef) {
		return errors.New("observation text fields cannot contain control characters")
	}
	if o.CheckedAt.IsZero() {
		return errors.New("checked_at is required")
	}
	if o.Duration < 0 {
		return errors.New("duration cannot be negative")
	}
	if o.Status != StatusHealthy && strings.TrimSpace(o.ReasonCode) == "" {
		return errors.New("reason_code is required for non-healthy observations")
	}
	if o.ReasonCode != "" && !validStableCode(o.ReasonCode) {
		return errors.New("reason_code must be a stable lowercase identifier")
	}
	if len(o.ReasonCode) > 160 || len(o.SafeMessage) > 512 || len(o.EvidenceRef) > 512 {
		return errors.New("observation field exceeds size limit")
	}
	if len(o.Attributes) > 16 {
		return errors.New("too many observation attributes")
	}
	total := 0
	for key, value := range o.Attributes {
		if strings.TrimSpace(key) == "" || len(key) > 64 || len(value) > 256 ||
			hasControlCharacters(key) || hasControlCharacters(value) {
			return errors.New("invalid observation attribute")
		}
		total += len(key) + len(value)
	}
	if total > 2048 {
		return errors.New("observation attributes exceed size limit")
	}
	return nil
}

// validStableCode accepts bounded machine identifiers, not free-form error text.
func validStableCode(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') ||
			(index > 0 && (char == '.' || char == '_' || char == '-')) {
			continue
		}
		return false
	}
	return true
}

func hasControlCharacters(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}
