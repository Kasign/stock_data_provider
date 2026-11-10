package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var (
	ErrChannelAdapterUnavailable = errors.New("notification: channel adapter unavailable")
	ErrChannelVersionNotFound    = errors.New("notification: channel config version not found")
	ErrChannelAlreadyRegistered  = errors.New("notification: channel config version already registered")
)

type channelKey struct {
	ref     string
	version int64
}

type channelRegistration struct {
	snapshot domain.ChannelConfigSnapshot
	adapter  ChannelAdapter
}

// ChannelRegistry stores immutable adapter registrations. It is an in-memory
// runtime registry, not the persistent Channel configuration Repository.
type ChannelRegistry struct {
	mu            sync.RWMutex
	registrations map[channelKey]channelRegistration
}

func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{registrations: make(map[channelKey]channelRegistration)}
}

func (r *ChannelRegistry) Register(adapter ChannelAdapter) error {
	if r == nil || adapter == nil {
		return ErrChannelAdapterUnavailable
	}
	snapshot := cloneChannelSnapshot(adapter.ConfigSnapshot())
	if err := snapshot.Validate(); err != nil {
		return err
	}
	sort.Strings(snapshot.RetryableErrorCodes)

	key := channelKey{ref: snapshot.ChannelRef, version: snapshot.ConfigVersion}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.registrations == nil {
		r.registrations = make(map[channelKey]channelRegistration)
	}
	if _, exists := r.registrations[key]; exists {
		return ErrChannelAlreadyRegistered
	}
	r.registrations[key] = channelRegistration{snapshot: snapshot, adapter: adapter}
	return nil
}

func (r *ChannelRegistry) Resolve(ctx context.Context, channelRef string, configVersion int64) (ChannelAdapter, error) {
	if r == nil || ctx == nil {
		return nil, ErrChannelAdapterUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	registration, exists := r.registrations[channelKey{ref: channelRef, version: configVersion}]
	r.mu.RUnlock()
	if !exists {
		return nil, ErrChannelVersionNotFound
	}
	return &boundChannelAdapter{
		snapshot: cloneChannelSnapshot(registration.snapshot),
		adapter:  registration.adapter,
	}, nil
}

type boundChannelAdapter struct {
	snapshot domain.ChannelConfigSnapshot
	adapter  ChannelAdapter
}

func (a *boundChannelAdapter) ConfigSnapshot() domain.ChannelConfigSnapshot {
	return cloneChannelSnapshot(a.snapshot)
}

func (a *boundChannelAdapter) Send(ctx context.Context, request domain.ChannelSendRequest) (domain.ChannelSendResult, error) {
	if a == nil || a.adapter == nil || ctx == nil {
		return domain.ChannelSendResult{}, ErrChannelAdapterUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domain.ChannelSendResult{
			Outcome: domain.ChannelOutcomeNotSent, ErrorCode: "context_cancelled",
		}, err
	}
	if err := request.Validate(a.snapshot); err != nil {
		return domain.ChannelSendResult{
			Outcome: domain.ChannelOutcomeNotSent, ErrorCode: "request_rejected",
		}, err
	}

	result, sendErr := a.adapter.Send(ctx, request)
	if err := result.Validate(a.snapshot.Capabilities); err != nil {
		ambiguous := domain.ChannelSendResult{
			Outcome: domain.ChannelOutcomeAmbiguous, ErrorCode: "adapter_invalid_result",
		}
		return ambiguous, fmt.Errorf("notification: adapter returned invalid send result: %w", err)
	}
	if sendErr != nil && result.Outcome == domain.ChannelOutcomeAccepted {
		ambiguous := domain.ChannelSendResult{
			Outcome: domain.ChannelOutcomeAmbiguous, ErrorCode: "adapter_error_after_send",
		}
		return ambiguous, sendErr
	}
	return result, sendErr
}

func cloneChannelSnapshot(snapshot domain.ChannelConfigSnapshot) domain.ChannelConfigSnapshot {
	cloned := snapshot
	cloned.RetryableErrorCodes = append([]string(nil), snapshot.RetryableErrorCodes...)
	return cloned
}

// EqualChannelSnapshot is used by tests and diagnostics to compare immutable
// registration metadata without exposing adapter implementation details.
func equalChannelSnapshot(left, right domain.ChannelConfigSnapshot) bool {
	left = cloneChannelSnapshot(left)
	right = cloneChannelSnapshot(right)
	sort.Strings(left.RetryableErrorCodes)
	sort.Strings(right.RetryableErrorCodes)
	return reflect.DeepEqual(left, right)
}
