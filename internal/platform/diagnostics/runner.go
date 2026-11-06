package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

const (
	defaultMaxConcurrent = 4
	maxConcurrentLimit   = 32
	defaultTotalTimeout  = 30 * time.Second
)

type Runner struct {
	MaxConcurrent int
	TotalTimeout  time.Duration
}

func (r Runner) Run(ctx context.Context, checks []Check) (Report, error) {
	if ctx == nil {
		return Report{}, errors.New("context is required")
	}
	if len(checks) > 128 {
		return Report{}, errors.New("at most 128 checks are allowed per run")
	}
	seen := make(map[string]struct{}, len(checks))
	for _, check := range checks {
		if err := check.validate(); err != nil {
			return Report{}, fmt.Errorf("check %q: %w", check.ID, err)
		}
		if _, exists := seen[check.ID]; exists {
			return Report{}, fmt.Errorf("duplicate check ID %q", check.ID)
		}
		seen[check.ID] = struct{}{}
	}

	workers := r.MaxConcurrent
	if workers <= 0 {
		workers = defaultMaxConcurrent
	}
	if workers > maxConcurrentLimit {
		return Report{}, fmt.Errorf("max concurrent checks cannot exceed %d", maxConcurrentLimit)
	}
	if workers > len(checks) {
		workers = len(checks)
	}
	if len(checks) == 0 {
		return Report{Summary: aggregate(nil), Results: []Result{}}, nil
	}

	totalTimeout := r.TotalTimeout
	if totalTimeout <= 0 {
		totalTimeout = defaultTotalTimeout
	}
	totalCtx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	results := make([]Result, len(checks))
	jobs := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for index := range jobs {
				results[index] = execute(totalCtx, checks[index])
			}
		}()
	}
	for i := range checks {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return Report{Summary: aggregate(results), Results: results}, nil
}

func execute(parent context.Context, check Check) (result Result) {
	result.CheckID = check.ID
	result.Name = check.Name
	result.Required = check.Required

	if err := parent.Err(); err != nil {
		result.Observation = notStartedObservation(err)
		return result
	}

	timeout := check.Timeout
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	started := time.Now()
	var observation Observation
	var runErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				// Panic payloads may contain credentials or upstream data; never retain them in Cause.
				_ = recovered
				runErr = errors.New("check panicked")
			}
		}()
		observation, runErr = check.Run(ctx)
	}()

	elapsed := time.Since(started)
	if runErr == nil && ctx.Err() != nil {
		runErr = ctx.Err()
	}
	if runErr != nil {
		f := normalizeFault(runErr, ctx.Err())
		result.Fault = f
		status := StatusFailed
		if f.Class == fault.ClassTimeout || f.Class == fault.ClassUnavailable {
			status = StatusUnavailable
		}
		result.Observation = Observation{
			Status: status, ReasonCode: f.Code, SafeMessage: f.SafeMessage,
			CheckedAt: time.Now().UTC(), Duration: elapsed,
		}
		return result
	}

	if observation.CheckedAt.IsZero() {
		observation.CheckedAt = time.Now().UTC()
	} else {
		observation.CheckedAt = observation.CheckedAt.UTC()
	}
	if observation.Duration <= 0 {
		observation.Duration = elapsed
	}
	if err := observation.Validate(); err != nil {
		f := fault.New(fault.ClassInternal, "platform.diagnostics.invalid_observation",
			"check returned an invalid observation", err)
		result.Fault = f
		result.Observation = Observation{
			Status: StatusFailed, ReasonCode: f.Code, SafeMessage: f.SafeMessage,
			CheckedAt: time.Now().UTC(), Duration: elapsed,
		}
		return result
	}
	if observation.Status == StatusSkipped && check.Required {
		f := fault.New(fault.ClassInternal, "platform.diagnostics.required_check_skipped",
			"required check cannot be skipped", nil)
		result.Fault = f
		observation.Status = StatusFailed
		observation.ReasonCode = f.Code
		observation.SafeMessage = f.SafeMessage
	}
	result.Observation = observation
	return result
}

func normalizeFault(err error, contextErr error) *fault.Fault {
	var f *fault.Fault
	if errors.As(err, &f) && f != nil {
		if !validFaultClass(f.Class) {
			return &fault.Fault{
				Class: fault.ClassInternal, Code: "platform.diagnostics.invalid_fault",
				SafeMessage: "check returned an invalid fault", Cause: err,
			}
		}
		copy := *f
		if copy.Code == "" {
			copy.Code = "platform.diagnostics.check_failed"
		}
		if copy.SafeMessage == "" {
			copy.SafeMessage = "check failed"
		}
		if len(copy.Code) > 160 || !validStableCode(copy.Code) ||
			len(copy.SafeMessage) > 512 || hasControlCharacters(copy.SafeMessage) ||
			len(copy.Dependency) > 128 || hasControlCharacters(copy.Dependency) {
			return &fault.Fault{
				Class: fault.ClassInternal, Code: "platform.diagnostics.invalid_fault",
				SafeMessage: "check returned an invalid fault", Cause: err,
			}
		}
		return &copy
	}
	if errors.Is(contextErr, context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &fault.Fault{
			Class: fault.ClassTimeout, Code: "platform.diagnostics.check_timeout",
			SafeMessage: "check timed out", Retryable: true, Cause: err,
		}
	}
	if errors.Is(contextErr, context.Canceled) || errors.Is(err, context.Canceled) {
		return &fault.Fault{
			Class: fault.ClassUnavailable, Code: "platform.diagnostics.check_canceled",
			SafeMessage: "check was canceled", Cause: err,
		}
	}
	return &fault.Fault{
		Class: fault.ClassInternal, Code: "platform.diagnostics.check_failed",
		SafeMessage: "check failed", Cause: err,
	}
}

func validFaultClass(class fault.Class) bool {
	switch class {
	case fault.ClassValidation, fault.ClassUnauthenticated, fault.ClassForbidden,
		fault.ClassNotFound, fault.ClassConflict, fault.ClassUnavailable,
		fault.ClassTimeout, fault.ClassInternal:
		return true
	default:
		return false
	}
}

func notStartedObservation(cause error) Observation {
	code := "platform.diagnostics.deadline_exceeded"
	message := "check was not completed before the execution deadline"
	if errors.Is(cause, context.Canceled) {
		code = "platform.diagnostics.canceled"
		message = "check was canceled before execution"
	}
	return Observation{
		Status: StatusUnavailable, ReasonCode: code, SafeMessage: message, CheckedAt: time.Now().UTC(),
	}
}
