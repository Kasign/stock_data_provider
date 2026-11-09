package application

import (
 "context"
 "sync"
)

// workerCancellationRegistry tracks local executions only; durable cancellation is persisted separately.
type workerCancellationRegistry struct {
 mu sync.Mutex
 byRun map[string]map[string]context.CancelCauseFunc
}

func newWorkerCancellationRegistry() *workerCancellationRegistry {
 return &workerCancellationRegistry{byRun: make(map[string]map[string]context.CancelCauseFunc)}
}

func (r *workerCancellationRegistry) register(runRef, attemptRef string, cancel context.CancelCauseFunc) func() {
 r.mu.Lock()
 if r.byRun[runRef] == nil { r.byRun[runRef] = make(map[string]context.CancelCauseFunc) }
 r.byRun[runRef][attemptRef] = cancel
 r.mu.Unlock()
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
 cancels := make([]context.CancelCauseFunc, 0, len(r.byRun[runRef]))
 for _, cancel := range r.byRun[runRef] { cancels = append(cancels, cancel) }
 r.mu.Unlock()
 for _, cancel := range cancels { cancel(ErrWorkerCancelled) }
 return len(cancels)
}
