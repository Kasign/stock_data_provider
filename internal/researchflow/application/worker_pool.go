package application

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var (
	ErrWorkerPoolNotRunning = errors.New("Researchflow Worker Pool is not running")
	ErrWorkerPoolStopping   = errors.New("Researchflow Worker Pool is stopping")
)

type WorkerPoolConfig struct {
	Workers int
}

type workerPoolState uint8

const (
	workerPoolCreated workerPoolState = iota
	workerPoolRunning
	workerPoolStopping
	workerPoolStopped
)

type workerPoolTask struct {
	ctx       context.Context
	runRef    string
	stageRef  string
	attemptRef string
	result    chan workerPoolResult
}

type workerPoolResult struct {
	execution WorkerExecution
	err       error
}

// WorkerPool 提供固定数量的执行协程；不持有持久化任务队列。
// 待执行 Stage 始终以数据库 Ready/RetryWait 状态为准，Worker Claim 仍是资源占用权威。
type WorkerPool struct {
	dispatcher StageExecutionDispatcher
	config     WorkerPoolConfig

	mu             sync.Mutex
	state          workerPoolState
	ctx            context.Context
	cancel         context.CancelFunc
	tasks          chan workerPoolTask
	stopAccepting  chan struct{}
	done           chan struct{}
	workers        sync.WaitGroup
}

func NewWorkerPool(dispatcher StageExecutionDispatcher, config WorkerPoolConfig) (*WorkerPool, error) {
	if dispatcher == nil || config.Workers < 1 {
		return nil, errors.New("Researchflow Worker Pool dependencies or configuration are invalid")
	}
	return &WorkerPool{dispatcher: dispatcher, config: config, state: workerPoolCreated}, nil
}

// Start 只允许启动一次；Worker 数量固定，任务通道不缓存任务。
func (p *WorkerPool) Start(ctx context.Context) error {
	if p == nil || ctx == nil {
		return errors.New("Researchflow Worker Pool and parent context are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != workerPoolCreated {
		return errors.New("Researchflow Worker Pool can only be started once")
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.tasks = make(chan workerPoolTask)
	p.stopAccepting = make(chan struct{})
	p.done = make(chan struct{})
	p.state = workerPoolRunning
	for index := 0; index < p.config.Workers; index++ {
		p.workers.Add(1)
		go p.workerLoop()
	}
	go func() {
		p.workers.Wait()
		p.mu.Lock()
		p.state = workerPoolStopped
		p.mu.Unlock()
		close(p.done)
	}()
	return nil
}

func (p *WorkerPool) workerLoop() {
	defer p.workers.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.stopAccepting:
			return
		case task := <-p.tasks:
			taskCtx, cancel := context.WithCancel(task.ctx)
			stopPoolCancellation := context.AfterFunc(p.ctx, cancel)
			execution, err := p.dispatcher.Execute(taskCtx, task.runRef, task.stageRef, task.attemptRef)
			stopPoolCancellation()
			cancel()
			task.result <- workerPoolResult{execution: execution, err: err}
		}
	}
}

// Execute 将单个 Stage 交给固定 Worker；通道无缓冲，满载时调用方自然背压。
func (p *WorkerPool) Execute(ctx context.Context, runRef, stageRef, attemptRef string) (WorkerExecution, error) {
	if p == nil || ctx == nil || strings.TrimSpace(runRef) == "" ||
		strings.TrimSpace(stageRef) == "" || strings.TrimSpace(attemptRef) == "" {
		return WorkerExecution{}, errors.New("Researchflow Worker Pool execution arguments are invalid")
	}
	p.mu.Lock()
	if p.state != workerPoolRunning {
		state := p.state
		p.mu.Unlock()
		if state == workerPoolStopping {
			return WorkerExecution{}, ErrWorkerPoolStopping
		}
		return WorkerExecution{}, ErrWorkerPoolNotRunning
	}
	poolCtx, tasks, stopAccepting := p.ctx, p.tasks, p.stopAccepting
	p.mu.Unlock()

	task := workerPoolTask{
		ctx: ctx, runRef: runRef, stageRef: stageRef, attemptRef: attemptRef,
		result: make(chan workerPoolResult, 1),
	}
	select {
	case tasks <- task:
	case <-ctx.Done():
		return WorkerExecution{}, ctx.Err()
	case <-poolCtx.Done():
		return WorkerExecution{}, ErrWorkerPoolNotRunning
	case <-stopAccepting:
		return WorkerExecution{}, ErrWorkerPoolStopping
	}
	select {
	case result := <-task.result:
		return result.execution, result.err
	case <-ctx.Done():
		return WorkerExecution{}, ctx.Err()
	case <-poolCtx.Done():
		return WorkerExecution{}, ErrWorkerPoolNotRunning
	}
}

// Shutdown 停止接收新任务并等待在途任务完成；超过调用方截止时间则取消池上下文。
func (p *WorkerPool) Shutdown(ctx context.Context) error {
	if p == nil || ctx == nil {
		return errors.New("Researchflow Worker Pool and shutdown context are required")
	}
	p.mu.Lock()
	switch p.state {
	case workerPoolCreated:
		p.state = workerPoolStopped
		p.mu.Unlock()
		return nil
	case workerPoolStopped:
		p.mu.Unlock()
		return nil
	case workerPoolRunning:
		p.state = workerPoolStopping
		close(p.stopAccepting)
	case workerPoolStopping:
	}
	done, cancel := p.done, p.cancel
	p.mu.Unlock()

	select {
	case <-done:
		cancel()
		return nil
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}

// Ensure WorkerPool remains a valid synchronous Stage dispatcher for Scheduler.
var _ StageExecutionDispatcher = (*WorkerPool)(nil)

