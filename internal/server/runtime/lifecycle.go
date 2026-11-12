// Package runtime 提供 Server 进程级组件生命周期管理。
package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Component 是由 Server 组合根管理的运行组件。
type Component interface {
	Name() string
	Start(context.Context) error
	Stop(context.Context) error
}

// Lifecycle 按注册顺序启动组件，并按逆序停止；启动失败时自动回滚。
type Lifecycle struct {
	mu         sync.Mutex
	components []Component
	started    []Component
	startedOK  bool
	stopped    bool
	startDone  chan struct{}
	stopDone   chan struct{}
	stopErr         error
	rollbackTimeout time.Duration
}

const defaultRollbackTimeout = 10 * time.Second

func NewLifecycle() *Lifecycle {
	return &Lifecycle{rollbackTimeout: defaultRollbackTimeout}
}

// NewLifecycleWithRollbackTimeout creates a Lifecycle whose startup rollback
// uses the same bounded shutdown budget as the owning Server process.
func NewLifecycleWithRollbackTimeout(timeout time.Duration) (*Lifecycle, error) {
	if timeout <= 0 {
		return nil, errors.New("lifecycle rollback timeout must be positive")
	}
	return &Lifecycle{rollbackTimeout: timeout}, nil
}

// Register 只允许在启动前注册组件，名称必须唯一。
func (l *Lifecycle) Register(component Component) error {
	if component == nil || component.Name() == "" {
		return errors.New("lifecycle component and name are required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.startedOK || l.stopped {
		return errors.New("cannot register component after lifecycle start")
	}
	for _, existing := range l.components {
		if existing.Name() == component.Name() {
			return fmt.Errorf("duplicate lifecycle component %q", component.Name())
		}
	}
	l.components = append(l.components, component)
	return nil
}

// Start 按注册顺序启动。并发 Stop 会等待整个启动阶段结束，避免组件
// 在 Start 尚未返回时被 Stop，或在 Stop 快照之后才加入 started 列表。
func (l *Lifecycle) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("lifecycle start context is required")
	}
	l.mu.Lock()
	if l.startedOK || l.stopped {
		l.mu.Unlock()
		return errors.New("lifecycle has already been started or stopped")
	}
	l.startedOK = true
	l.startDone = make(chan struct{})
	components := append([]Component(nil), l.components...)
	l.mu.Unlock()

	startErr := l.startComponents(ctx, components)

	l.mu.Lock()
	close(l.startDone)
	l.mu.Unlock()

	if startErr != nil {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), l.rollbackTimeout)
		defer cancel()
		rollbackErr := l.Stop(rollbackCtx)
		return errors.Join(startErr, rollbackErr)
	}
	return nil
}

func (l *Lifecycle) startComponents(ctx context.Context, components []Component) error {
	for _, component := range components {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("lifecycle startup canceled: %w", err)
		}
		l.mu.Lock()
		if l.stopped {
			l.mu.Unlock()
			return errors.New("lifecycle startup interrupted by stop")
		}
		l.started = append(l.started, component)
		l.mu.Unlock()
		if err := component.Start(ctx); err != nil {
			return fmt.Errorf("start component %q: %w", component.Name(), err)
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("lifecycle startup canceled: %w", err)
	}
	return nil
}

// Stop 等待正在进行的启动阶段结束，再按启动逆序停止；重复调用安全。
func (l *Lifecycle) Stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("lifecycle stop context is required")
	}
	l.mu.Lock()
	if l.stopped {
		done := l.stopDone
		l.mu.Unlock()
		select {
		case <-done:
			l.mu.Lock()
			err := l.stopErr
			l.mu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	l.stopped = true
	l.stopDone = make(chan struct{})
	done := l.stopDone
	startDone := l.startDone
	l.mu.Unlock()

	if startDone != nil {
		// Start 使用进程 Context；必须等它退出后才能安全关闭共享资源。
		// 否则 Bootstrap 可能在组件仍处于 Start 时继续关闭 DB/Redis。
		<-startDone
	}

	l.mu.Lock()
	started := append([]Component(nil), l.started...)
	l.started = nil
	l.mu.Unlock()

	var result error
	for i := len(started) - 1; i >= 0; i-- {
		component := started[i]
		if err := component.Stop(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("stop component %q: %w", component.Name(), err))
		}
	}

	l.mu.Lock()
	l.stopErr = result
	close(done)
	l.mu.Unlock()
	return result
}
