package application

import (
 "context"
 "sync"
)

// workerCancellationRegistry tracks local executions only; durable cancellation is persisted separately.
type workerCancellationRegistry struct {
 mu sync.Mutex
 byRun map[string]map[string]context.CancelCauseFunc
	cancelledRuns map[string]struct{}
}

func newWorkerCancellationRegistry() *workerCancellationRegistry {
 return &workerCancellationRegistry{byRun: make(map[string]map[string]context.CancelCauseFunc), cancelledRuns: make(map[string]struct{})}
}

func (r *workerCancellationRegistry) register(runRef, attemptRef string, cancel context.CancelCauseFunc) func() {
 r.mu.Lock()
 if r.byRun[runRef] == nil { r.byRun[runRef] = make(map[string]context.CancelCauseFunc) }
 r.byRun[runRef][attemptRef] = cancel
 _, alreadyCancelled := r.cancelledRuns[runRef]
 r.mu.Unlock()
 if alreadyCancelled { cancel(ErrWorkerCancelled) }
 return func() {
  r.mu.Lock()
  defer r.mu.Unlock()
  attempts := r.byRun[runRef]
  delete(attempts, attemptRef)
  if len(attempts) == 0 { delete(r.byRun, runRef) }
 }
}

func (r *workerCancellationRegistry) cancelRun(runRef string) int {
 r.mu.Lock()
 r.cancelledRuns[runRef] = struct{}{}
 cancels := make([]context.CancelCauseFunc, 0, len(r.byRun[runRef]))
 for _, cancel := range r.byRun[runRef] { cancels = append(cancels, cancel) }
 r.mu.Unlock()
 for _, cancel := range cancels { cancel(ErrWorkerCancelled) }
 return len(cancels)
}
