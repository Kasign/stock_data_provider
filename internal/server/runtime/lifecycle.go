// Package runtime 提供 Server 进程级组件生命周期管理。
package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
}

func NewLifecycle() *Lifecycle { return &Lifecycle{} }

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

// Start 按注册顺序启动；任一组件失败都会停止已尝试启动的组件。
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
	components := append([]Component(nil), l.components...)
	l.mu.Unlock()
	for _, component := range components {
		l.mu.Lock()
		l.started = append(l.started, component)
		l.mu.Unlock()
		if err := component.Start(ctx); err != nil {
			rollbackErr := l.Stop(context.WithoutCancel(ctx))
			return errors.Join(fmt.Errorf("start component %q: %w", component.Name(), err), rollbackErr)
		}
	}
	return nil
}

// Stop 按启动逆序停止，重复调用安全；返回所有停止错误。
func (l *Lifecycle) Stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("lifecycle stop context is required")
	}
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return nil
	}
	l.stopped = true
	started := append([]Component(nil), l.started...)
	l.started = nil
	l.mu.Unlock()
	var result error
	for i := len(started)-1; i >= 0; i-- {
		component := started[i]
		if err := component.Stop(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("stop component %q: %w", component.Name(), err))
		}
	}
	return result
}
