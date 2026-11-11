package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type FanoutWorkItemStatus string

const (
	FanoutPending    FanoutWorkItemStatus = "pending"
	FanoutProcessing FanoutWorkItemStatus = "processing"
	FanoutRetryWait  FanoutWorkItemStatus = "retry_wait"
	FanoutCompleted  FanoutWorkItemStatus = "completed"
	FanoutFailed     FanoutWorkItemStatus = "failed"
)

var ErrInvalidFanoutWorkItem = errors.New("notification: invalid fanout work item")

// FanoutWorkItem 是一个已接收事件的唯一展开任务；事件正文仍以 Inbox 为事实源。
type FanoutWorkItem struct {
	WorkItemID           string
	SourceModule         string
	EventID              string
	EventType            string
	PayloadFingerprint   string
	SnapshotRevision     int64
	HasSnapshot          bool
	CursorSubscriptionID string
	Status               FanoutWorkItemStatus
	AttemptCount         int64
	LeaseWorkerID        string
	LeaseExpiresAt       time.Time
	FencingToken         int64
	NextAttemptAt        time.Time
	LastErrorCode        string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// FanoutWorkItemID 为同一 Inbox 身份生成稳定内部 ID，不依赖随机数或进程状态。
func FanoutWorkItemID(sourceModule, eventID string) string {
	sum := sha256.Sum256([]byte(sourceModule + "\x00" + eventID))
	return "ntf-fw-" + hex.EncodeToString(sum[:])
}

func isLowerHexDigest(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}

func (w FanoutWorkItem) Validate() error {
	if w.WorkItemID == "" || w.WorkItemID != FanoutWorkItemID(w.SourceModule, w.EventID) ||
		w.SourceModule == "" || w.EventID == "" || w.EventType == "" ||
		len(w.PayloadFingerprint) != 64 || strings.ToLower(w.PayloadFingerprint) != w.PayloadFingerprint || !isLowerHexDigest(w.PayloadFingerprint) ||
		w.AttemptCount < 0 || w.FencingToken < 0 || w.SnapshotRevision < 0 ||
		w.CreatedAt.IsZero() || w.UpdatedAt.IsZero() || w.UpdatedAt.Before(w.CreatedAt) {
		return ErrInvalidFanoutWorkItem
	}
	switch w.Status {
	case FanoutPending, FanoutRetryWait, FanoutProcessing, FanoutCompleted, FanoutFailed:
	default:
		return ErrInvalidFanoutWorkItem
	}
	if w.HasSnapshot && w.SnapshotRevision < 0 {
		return ErrInvalidFanoutWorkItem
	}
	if w.Status == FanoutProcessing {
		if w.LeaseWorkerID == "" || w.LeaseExpiresAt.IsZero() || w.FencingToken < 1 {
			return ErrInvalidFanoutWorkItem
		}
	} else if w.LeaseWorkerID != "" || !w.LeaseExpiresAt.IsZero() {
		return ErrInvalidFanoutWorkItem
	}
	if w.Status == FanoutRetryWait {
		if w.NextAttemptAt.IsZero() {
			return ErrInvalidFanoutWorkItem
		}
	} else if !w.NextAttemptAt.IsZero() {
		return ErrInvalidFanoutWorkItem
	}
	return nil
}
