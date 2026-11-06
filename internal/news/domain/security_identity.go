package domain

import (
	"errors"
	"strings"
	"time"
)

// SecuritySymbolStatus 表示证券代码在其有效期内的身份状态。
type SecuritySymbolStatus string

const (
	SecuritySymbolActive     SecuritySymbolStatus = "active"
	SecuritySymbolHistorical SecuritySymbolStatus = "historical"
)

// SecurityLifecycleStatus 表示证券本身的上市生命周期状态。
type SecurityLifecycleStatus string

const (
	SecurityLifecycleListed   SecurityLifecycleStatus = "listed"
	SecurityLifecycleDelisted SecurityLifecycleStatus = "delisted"
)

// SecurityIdentityRecord 是来自外部权威证券目录的时间有效身份快照。
// IdentityNamespace + IdentityValue 必须是上游稳定的证券级身份键，不得使用公司/发行人 ID 替代。
type SecurityIdentityRecord struct {
	IdentityNamespace string
	IdentityValue     string
	Market            string
	Symbol            string
	Name              string
	SymbolStatus      SecuritySymbolStatus
	LifecycleStatus   SecurityLifecycleStatus
	ValidFrom         time.Time
	ValidTo           *time.Time
	ObservedAt        time.Time
	Source            string
	EvidenceRef       string
}

type SecurityIdentityResolutionStatus string

const (
	SecurityIdentityResolved  SecurityIdentityResolutionStatus = "resolved"
	SecurityIdentityNotFound  SecurityIdentityResolutionStatus = "not_found"
	SecurityIdentityAmbiguous SecurityIdentityResolutionStatus = "ambiguous"
)

type SecurityIdentityResolution struct {
	Status SecurityIdentityResolutionStatus
	Record SecurityIdentityRecord
}

var (
	ErrSecurityIdentityInvalid  = errors.New("news.security_identity.invalid")
	ErrSecurityIdentityConflict = errors.New("news.security_identity.conflict")
)

// ValidateSecurityIdentityRecord 校验证券身份快照；不允许缺少来源证据或有效起始时间。
func ValidateSecurityIdentityRecord(record SecurityIdentityRecord) error {
	if strings.TrimSpace(record.IdentityNamespace) == "" ||
		strings.TrimSpace(record.IdentityValue) == "" ||
		strings.TrimSpace(record.Market) == "" ||
		strings.TrimSpace(record.Symbol) == "" ||
		record.ValidFrom.IsZero() ||
		record.ObservedAt.IsZero() ||
		strings.TrimSpace(record.Source) == "" ||
		strings.TrimSpace(record.EvidenceRef) == "" {
		return ErrSecurityIdentityInvalid
	}
	if record.ValidTo != nil && !record.ValidTo.After(record.ValidFrom) {
		return ErrSecurityIdentityInvalid
	}
	if record.SymbolStatus != SecuritySymbolActive && record.SymbolStatus != SecuritySymbolHistorical {
		return ErrSecurityIdentityInvalid
	}
	if record.LifecycleStatus != SecurityLifecycleListed && record.LifecycleStatus != SecurityLifecycleDelisted {
		return ErrSecurityIdentityInvalid
	}
	return nil
}

// ResolveSecurityIdentity 按市场、代码和业务有效时间解析身份。
// 历史代码仍可在其有效期内解析；同一时点出现多个匹配记录时返回 ambiguous，不猜测归属。
func ResolveSecurityIdentity(records []SecurityIdentityRecord, market, symbol string, asOf time.Time) (SecurityIdentityResolution, error) {
	market, symbol = strings.TrimSpace(market), strings.TrimSpace(symbol)
	if market == "" || symbol == "" || asOf.IsZero() {
		return SecurityIdentityResolution{}, ErrSecurityIdentityInvalid
	}

	var matched *SecurityIdentityRecord
	for i := range records {
		record := records[i]
		if err := ValidateSecurityIdentityRecord(record); err != nil {
			return SecurityIdentityResolution{}, err
		}
		if record.Market != market || record.Symbol != symbol || asOf.Before(record.ValidFrom) ||
			(record.ValidTo != nil && !asOf.Before(*record.ValidTo)) {
			continue
		}
		if matched != nil {
			return SecurityIdentityResolution{Status: SecurityIdentityAmbiguous}, nil
		}
		copy := record
		matched = &copy
	}
	if matched == nil {
		return SecurityIdentityResolution{Status: SecurityIdentityNotFound}, nil
	}
	return SecurityIdentityResolution{Status: SecurityIdentityResolved, Record: *matched}, nil
}
