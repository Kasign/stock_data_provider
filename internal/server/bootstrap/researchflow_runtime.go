package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
)

// researchflowRuntimeComponent owns only the in-process execution lifecycle.
// Its durable state, admission limits and recovery decisions remain in MariaDB.
type researchflowRuntimeComponent struct {
	pool                  *researchflowapplication.WorkerPool
	ids                   researchflowapplication.IDGenerator
	scheduler              *researchflowapplication.Scheduler
	schedulerCandidates    *researchflowapplication.SchedulerCandidateScanner
	expiredAttempts        *researchflowapplication.ExpiredAttemptScanner
	deadlines              *researchflowapplication.DeadlineScanner
	cancellations          *researchflowapplication.CancellationRecoveryScanner
	finalizationCandidates *researchflowapplication.RunFinalizationCandidateScanner
	finalizer              *researchflowapplication.RunFinalizationCoordinator

	logger *slog.Logger
	cancel context.CancelFunc
	done   chan struct{}

	schedulerCursor    string
	expiredRunCursor   string
	expiredStageCursor string
	cancelRunCursor    string
	cancelStageCursor  string
	finalizerCursor    string
}

func newResearchflowRuntimeComponent(
	composition *researchflowComposition,
	capacity serverconfig.ResearchflowCapacity,
	localWorkers int,
) (*researchflowRuntimeComponent, error) {
	if composition == nil || composition.runRepository == nil || composition.newsStage == nil ||
		composition.clock == nil || composition.ids == nil || !capacity.Configured ||
		capacity.MaxWorkers <= 0 || localWorkers <= 0 {
		return nil, errors.New("Researchflow Runtime composition or capacity is invalid")
	}

	workerID, err := composition.ids.NewRef(context.Background(), "worker")
	if err != nil {
		return nil, fmt.Errorf("generate process-unique Researchflow Worker ID: %w", err)
	}
	worker, err := researchflowapplication.NewWorkerWithStageOutputRegistry(
		composition.runRepository,
		composition.newsStage.executor,
		composition.clock,
		researchflowapplication.WorkerConfig{
			WorkerID: workerID, LeaseDuration: 30 * time.Second,
			HeartbeatInterval: 10 * time.Second, MaxParallel: capacity.MaxWorkers,
		},
		composition.newsStage.registry,
	)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Worker: %w", err)
	}
	pool, err := researchflowapplication.NewWorkerPool(worker, researchflowapplication.WorkerPoolConfig{Workers: localWorkers})
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Worker Pool: %w", err)
	}

	scheduler, err := researchflowapplication.NewScheduler(
		composition.runRepository, composition.runRepository, composition.clock,
		composition.ids, nil, pool,
		researchflowapplication.SchedulerConfig{MaxParallel: capacity.MaxWorkers, MaxReplans: 3},
	)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Scheduler: %w", err)
	}
	schedulerCandidates, err := researchflowapplication.NewSchedulerCandidateScanner(composition.runRepository, 100)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Scheduler Candidate Scanner: %w", err)
	}
	expiredAttempts, err := researchflowapplication.NewExpiredAttemptScanner(composition.runRepository, 100)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Expired Attempt Scanner: %w", err)
	}
	deadlines, err := researchflowapplication.NewDeadlineScanner(
		composition.runRepository, researchflowapplication.DeadlineScanConfig{BatchSize: 100},
	)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Deadline Scanner: %w", err)
	}
	cancellations, err := researchflowapplication.NewCancellationRecoveryScanner(composition.runRepository, 100)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Cancellation Recovery Scanner: %w", err)
	}
	finalizationCandidates, err := researchflowapplication.NewRunFinalizationCandidateScanner(composition.runRepository, 100)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Finalization Candidate Scanner: %w", err)
	}

	normalFinalizer, err := researchflowapplication.NewRunFinalizer(
		composition.runRepository, composition.clock, composition.ids,
		researchflowapplication.RunFinalizerConfig{SummarySchemaVersion: "1.0.0"},
	)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Run Finalizer: %w", err)
	}
	deadlineFinalizer, err := researchflowapplication.NewDeadlineFinalizer(
		composition.runRepository, composition.clock, composition.ids,
		researchflowapplication.DeadlineFinalizerConfig{SummarySchemaVersion: "1.0.0"},
	)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Deadline Finalizer: %w", err)
	}
	cancellationFinalizer, err := researchflowapplication.NewCancellationFinalizer(
		composition.runRepository, composition.clock, composition.ids,
		researchflowapplication.CancellationFinalizerConfig{SummarySchemaVersion: "1.0.0"},
	)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Cancellation Finalizer: %w", err)
	}
	finalizer, err := researchflowapplication.NewRunFinalizationCoordinator(
		composition.runRepository, normalFinalizer, deadlineFinalizer, cancellationFinalizer,
	)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Run Finalization Coordinator: %w", err)
	}

	return &researchflowRuntimeComponent{
		pool: pool, ids: composition.ids, scheduler: scheduler,
		schedulerCandidates: schedulerCandidates, expiredAttempts: expiredAttempts,
		deadlines: deadlines, cancellations: cancellations,
		finalizationCandidates: finalizationCandidates, finalizer: finalizer,
		logger: slog.Default(), done: make(chan struct{}),
	}, nil
}

func (c *researchflowRuntimeComponent) Name() string { return "researchflow-runtime" }

func (c *researchflowRuntimeComponent) Start(ctx context.Context) error {
	if c == nil || ctx == nil || c.pool == nil || c.done == nil {
		return errors.New("Researchflow Runtime and start context are required")
	}
	if err := c.pool.Start(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("start Researchflow Worker Pool: %w", err)
	}
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c.cancel = cancel
	go c.run(loopCtx)
	return nil
}

func (c *researchflowRuntimeComponent) Stop(ctx context.Context) error {
	if c == nil || ctx == nil {
		return errors.New("Researchflow Runtime and stop context are required")
	}
	if c.cancel != nil {
		c.cancel()
	}
	select {
	case <-c.done:
	case <-ctx.Done():
		return fmt.Errorf("wait for Researchflow scan loop: %w", ctx.Err())
	}
	if err := c.pool.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown Researchflow Worker Pool: %w", err)
	}
	return nil
}

func (c *researchflowRuntimeComponent) run(ctx context.Context) {
	defer close(c.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		c.scanCycle(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *researchflowRuntimeComponent) scanCycle(ctx context.Context) {
	if _, err := c.deadlines.ScanBatch(ctx); err != nil && ctx.Err() == nil {
		c.logger.Error("Researchflow Deadline 扫描失败", "error", err)
	}
	c.scanCancellationRecovery(ctx)
	c.scanExpiredAttempts(ctx)
	c.scanSchedulableRuns(ctx)
	c.scanFinalizableRuns(ctx)
}

func (c *researchflowRuntimeComponent) scanCancellationRecovery(ctx context.Context) {
	for pageCount := 0; pageCount < 10; pageCount++ {
		page, err := c.cancellations.ListNextPage(ctx, c.cancelRunCursor, c.cancelStageCursor)
		if err != nil {
			if ctx.Err() == nil { c.logger.Error("Researchflow Cancellation Recovery 候选扫描失败", "error", err) }
			return
		}
		for _, candidate := range page.Candidates {
			one := researchflowapplication.CancellationRecoveryPage{Candidates: []researchflowapplication.ExpiredCancellationAttemptCandidate{candidate}}
			if _, err := c.cancellations.RecoverPage(ctx, one); err != nil && ctx.Err() == nil {
				c.logger.Error("Researchflow Cancellation Attempt 恢复失败", "run_ref", candidate.RunRef, "stage_ref", candidate.StageRef, "error", err)
			}
		}
		if !page.HasMore { c.cancelRunCursor, c.cancelStageCursor = "", ""; return }
		c.cancelRunCursor, c.cancelStageCursor = page.NextRunRef, page.NextStageRef
	}
}

func (c *researchflowRuntimeComponent) scanExpiredAttempts(ctx context.Context) {
	for pageCount := 0; pageCount < 10; pageCount++ {
		page, err := c.expiredAttempts.ListNextPage(ctx, c.expiredRunCursor, c.expiredStageCursor)
		if err != nil {
			if ctx.Err() == nil { c.logger.Error("Researchflow Expired Attempt 候选扫描失败", "error", err) }
			return
		}
		for _, candidate := range page.Candidates {
			attemptRef, idErr := c.ids.NewRef(ctx, "attempt")
			if idErr != nil {
				if ctx.Err() == nil { c.logger.Error("生成 Researchflow 恢复 Attempt ID 失败", "error", idErr) }
				continue
			}
			if _, executeErr := c.pool.Execute(ctx, candidate.RunRef, candidate.StageRef, attemptRef); executeErr != nil && ctx.Err() == nil {
				c.logger.Error("Researchflow Expired Attempt 恢复执行失败", "run_ref", candidate.RunRef, "stage_ref", candidate.StageRef, "error", executeErr)
			}
		}
		if !page.HasMore { c.expiredRunCursor, c.expiredStageCursor = "", ""; return }
		c.expiredRunCursor, c.expiredStageCursor = page.NextRunRef, page.NextStageRef
	}
}

func (c *researchflowRuntimeComponent) scanSchedulableRuns(ctx context.Context) {
	for pageCount := 0; pageCount < 10; pageCount++ {
		page, err := c.schedulerCandidates.ListNextPage(ctx, c.schedulerCursor)
		if err != nil {
			if ctx.Err() == nil { c.logger.Error("Researchflow Scheduler 候选扫描失败", "error", err) }
			return
		}
		for _, runRef := range page.RunRefs {
			if _, scheduleErr := c.scheduler.ScheduleRun(ctx, runRef); scheduleErr != nil && ctx.Err() == nil {
				c.logger.Error("Researchflow Run 调度失败", "run_ref", runRef, "error", scheduleErr)
			}
		}
		if !page.HasMore { c.schedulerCursor = ""; return }
		c.schedulerCursor = page.NextCursor
	}
}

func (c *researchflowRuntimeComponent) scanFinalizableRuns(ctx context.Context) {
	for pageCount := 0; pageCount < 10; pageCount++ {
		page, err := c.finalizationCandidates.ListNextPage(ctx, c.finalizerCursor)
		if err != nil {
			if ctx.Err() == nil { c.logger.Error("Researchflow Run Finalization 候选扫描失败", "error", err) }
			return
		}
		for _, runRef := range page.RunRefs {
			if _, finalizeErr := c.finalizer.Finalize(ctx, runRef); finalizeErr != nil && ctx.Err() == nil {
				c.logger.Error("Researchflow Run Finalization 失败", "run_ref", runRef, "error", finalizeErr)
			}
		}
		if !page.HasMore { c.finalizerCursor = ""; return }
		c.finalizerCursor = page.NextCursor
	}
}
