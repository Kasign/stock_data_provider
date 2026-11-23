package application

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

var (
	ErrInvalidPolicyChange = errors.New("invalid Quant result access policy change")
	ErrPolicyVersionConflict = errors.New("Quant result access policy version conflict")
	ErrPolicyOwnerUnavailable = errors.New("Quant result access policy owner unavailable")
)

// ResultAccessPolicyState is the current immutable Owner policy version.
// Version zero is the fail-closed bootstrap state and has no snapshot.
type ResultAccessPolicyState struct {
	Version  uint64
	Enabled  bool
	Snapshot *PolicySnapshot
}

// ResultAccessPolicyStore persists immutable policy versions and atomically
// advances the current pointer. Implementations serialize updates and return
// ErrPolicyVersionConflict when expectedVersion is stale.
type ResultAccessPolicyStore interface {
	CurrentResultAccessPolicy(ctx context.Context) (ResultAccessPolicyState, error)
	SetResultAccessPolicyEnabled(ctx context.Context, actorRef string, expectedVersion uint64, enabled bool) (ResultAccessPolicyState, error)
}

// ResultAccessPolicyManager is the Quant Owner application use case.
// Authentication is deliberately supplied by Server; actorRef must be the
// trusted Owner Session subject, never a request-body identity.
type ResultAccessPolicyManager struct {
	store ResultAccessPolicyStore
}

func NewResultAccessPolicyManager(store ResultAccessPolicyStore) (*ResultAccessPolicyManager, error) {
	if store == nil {
		return nil, ErrInvalidPolicyChange
	}
	return &ResultAccessPolicyManager{store: store}, nil
}

func (manager *ResultAccessPolicyManager) Current(ctx context.Context) (ResultAccessPolicyState, error) {
	if manager == nil || manager.store == nil || ctx == nil {
		return ResultAccessPolicyState{}, ErrInvalidPolicyChange
	}
	if err := ctx.Err(); err != nil {
		return ResultAccessPolicyState{}, err
	}
	state, err := manager.store.CurrentResultAccessPolicy(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ResultAccessPolicyState{}, ctxErr
		}
		return ResultAccessPolicyState{}, ErrPolicyOwnerUnavailable
	}
	if err := validateResultAccessPolicyState(state); err != nil {
		return ResultAccessPolicyState{}, ErrPolicyOwnerUnavailable
	}
	return cloneResultAccessPolicyState(state), nil
}

func (manager *ResultAccessPolicyManager) Publish(ctx context.Context, actorRef string, expectedVersion uint64) (ResultAccessPolicyState, error) {
	return manager.setEnabled(ctx, actorRef, expectedVersion, true)
}

func (manager *ResultAccessPolicyManager) Revoke(ctx context.Context, actorRef string, expectedVersion uint64) (ResultAccessPolicyState, error) {
	return manager.setEnabled(ctx, actorRef, expectedVersion, false)
}

func (manager *ResultAccessPolicyManager) setEnabled(ctx context.Context, actorRef string, expectedVersion uint64, enabled bool) (ResultAccessPolicyState, error) {
	if manager == nil || manager.store == nil || ctx == nil || actorRef != "owner" || strings.TrimSpace(actorRef) != actorRef {
		return ResultAccessPolicyState{}, ErrInvalidPolicyChange
	}
	if err := ctx.Err(); err != nil {
		return ResultAccessPolicyState{}, err
	}
	state, err := manager.store.SetResultAccessPolicyEnabled(ctx, actorRef, expectedVersion, enabled)
	if err != nil {
		if errors.Is(err, ErrPolicyVersionConflict) {
			return ResultAccessPolicyState{}, ErrPolicyVersionConflict
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ResultAccessPolicyState{}, ctxErr
		}
		return ResultAccessPolicyState{}, ErrPolicyOwnerUnavailable
	}
	if err := validateResultAccessPolicyState(state); err != nil {
		return ResultAccessPolicyState{}, ErrPolicyOwnerUnavailable
	}
	if state.Enabled != enabled || (enabled && state.Version == 0) {
		return ResultAccessPolicyState{}, ErrPolicyOwnerUnavailable
	}
	return cloneResultAccessPolicyState(state), nil
}

func validateResultAccessPolicyState(state ResultAccessPolicyState) error {
	if state.Version == 0 {
		if state.Enabled || state.Snapshot != nil {
			return ErrInvalidPolicyChange
		}
		return nil
	}
	if state.Snapshot == nil || state.Snapshot.SchemaVersion != "1.0.0" ||
		state.Snapshot.SnapshotRef == "" || len(state.Snapshot.Policies) != 1 {
		return ErrInvalidPolicyChange
	}
	policy := state.Snapshot.Policies[0]
	if policy.PolicyType != quantResultAccessPolicyType || policy.PolicyRef != quantResultAccessPolicyRef ||
		policy.PolicyVersion != strconv.FormatUint(state.Version, 10) {
		return ErrInvalidPolicyChange
	}
	return nil
}

func cloneResultAccessPolicyState(state ResultAccessPolicyState) ResultAccessPolicyState {
	if state.Snapshot != nil {
		snapshot := *state.Snapshot
		snapshot.Policies = append([]PolicyVersion{}, state.Snapshot.Policies...)
		state.Snapshot = &snapshot
	}
	return state
}
