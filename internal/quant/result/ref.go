package result

import (
	"errors"
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"

	"example.com/stock-analysis-flow/internal/quant/manifest"
)

var (
	ErrInvalidResultRef = errors.New("invalid quant result reference")
	refSchemaVersionRE  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	tradeDateRE         = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

type QualityStatus string

const (
	QualityValid    QualityStatus = "valid"
	QualityDegraded QualityStatus = "degraded"
	QualityInvalid  QualityStatus = "invalid"
	QualityUnknown  QualityStatus = "unknown"
)

type ResultStatus string

const (
	StatusCompleted          ResultStatus = "completed"
	StatusInsufficientData   ResultStatus = "insufficient_data"
	StatusInsufficientSample ResultStatus = "insufficient_sample"
	StatusRejected           ResultStatus = "rejected"
	StatusFailed             ResultStatus = "failed"
)

type Scope string

const (
	ScopeStock     Scope = "stock"
	ScopeMarket    Scope = "market"
	ScopeIndustry  Scope = "industry"
	ScopePortfolio Scope = "portfolio"
)

// Ref 是 QuantResult 的轻量引用投影，不包含 payload。
// result_id、result_type、system_as_of 和 input_snapshot_refs 均从已验证 Manifest 派生。
type Ref struct {
	SchemaVersion        string        `json:"schema_version"`
	ResultID             string        `json:"result_id"`
	ResultType           string        `json:"result_type"`
	Scope                Scope         `json:"scope"`
	SecurityID           string        `json:"security_id,omitempty"`
	SystemAsOf           string        `json:"system_as_of"`
	TradeDate            string        `json:"trade_date,omitempty"`
	Period               string        `json:"period"`
	AlgorithmVersion     string        `json:"algorithm_version"`
	ParameterFingerprint string        `json:"parameter_fingerprint"`
	InputSnapshotRefs    []string      `json:"input_snapshot_refs"`
	QualityStatus        QualityStatus `json:"quality_status"`
	ResultStatus         ResultStatus  `json:"result_status"`
	AuthorizationRef     string        `json:"authorization_ref"`
	CreatedAt            string        `json:"created_at"`
}

type RefInput struct {
	// SchemaVersion 是 QuantResultRef DTO 自身版本，不是 Manifest 的 payload schema_version。
	SchemaVersion        string
	Scope                Scope
	SecurityID           string
	TradeDate            string
	Period               string
	AlgorithmVersion     string
	ParameterFingerprint string
	QualityStatus        QualityStatus
	ResultStatus         ResultStatus
	AuthorizationRef     string
	CreatedAt            time.Time
}

// BuildRef 仅从已验证 Manifest 构造引用；不签发或验证 AuthorizationRef 的授权事实。
func BuildRef(input RefInput, source manifest.Manifest) (Ref, error) {
	if err := source.Verify(); err != nil {
		return Ref{}, fmt.Errorf("%w: manifest: %v", ErrInvalidResultRef, err)
	}
	if !refSchemaVersionRE.MatchString(input.SchemaVersion) ||
		!utf8.ValidString(input.SecurityID) ||
		!utf8.ValidString(input.Period) ||
		!utf8.ValidString(input.AlgorithmVersion) ||
		!utf8.ValidString(input.ParameterFingerprint) ||
		!utf8.ValidString(input.AuthorizationRef) ||
		!validSnapshotRefsUTF8(source.InputSnapshotRefs) ||
		!validScope(input.Scope) ||
		(input.Scope == ScopeStock && input.SecurityID == "") ||
		input.Period == "" || input.AlgorithmVersion == "" ||
		input.ParameterFingerprint == "" || !validQuality(input.QualityStatus) ||
		!validResultStatus(input.ResultStatus) || input.AuthorizationRef == "" ||
		input.CreatedAt.IsZero() {
		return Ref{}, ErrInvalidResultRef
	}
	if input.TradeDate != "" {
		if !tradeDateRE.MatchString(input.TradeDate) {
			return Ref{}, fmt.Errorf("%w: trade_date format", ErrInvalidResultRef)
		}
		if _, err := time.Parse("2006-01-02", input.TradeDate); err != nil {
			return Ref{}, fmt.Errorf("%w: trade_date value", ErrInvalidResultRef)
		}
	}

	return Ref{
		SchemaVersion:        input.SchemaVersion,
		ResultID:             source.ResultID,
		ResultType:           source.ResultType,
		Scope:                input.Scope,
		SecurityID:           input.SecurityID,
		SystemAsOf:           source.SystemAsOf,
		TradeDate:            input.TradeDate,
		Period:               input.Period,
		AlgorithmVersion:     input.AlgorithmVersion,
		ParameterFingerprint: input.ParameterFingerprint,
		InputSnapshotRefs:    append([]string(nil), source.InputSnapshotRefs...),
		QualityStatus:        input.QualityStatus,
		ResultStatus:         input.ResultStatus,
		AuthorizationRef:     input.AuthorizationRef,
		CreatedAt:            input.CreatedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

func validSnapshotRefsUTF8(refs []string) bool {
	for _, ref := range refs {
		if !utf8.ValidString(ref) {
			return false
		}
	}
	return true
}

func validScope(scope Scope) bool {
	switch scope {
	case ScopeStock, ScopeMarket, ScopeIndustry, ScopePortfolio:
		return true
	default:
		return false
	}
}

func validQuality(status QualityStatus) bool {
	switch status {
	case QualityValid, QualityDegraded, QualityInvalid, QualityUnknown:
		return true
	default:
		return false
	}
}

func validResultStatus(status ResultStatus) bool {
	switch status {
	case StatusCompleted, StatusInsufficientData, StatusInsufficientSample, StatusRejected, StatusFailed:
		return true
	default:
		return false
	}
}
