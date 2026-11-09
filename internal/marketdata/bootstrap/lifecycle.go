package bootstrap

import (
	"context"
	"errors"
	"time"
)

var errModuleClosed = errors.New("market-data module is closed")

// Start 启动 Module 管理的后台任务；重复调用不会重复启动 Worker。
func (m *Module) Start(ctx context.Context) error {
	if m == nil {
		return errors.New("market-data module is nil")
	}
	if ctx == nil {
		return errors.New("context must not be nil")
	}

	m.lifecycleMu.Lock()
	if m.closed || m.closing {
		m.lifecycleMu.Unlock()
		return errModuleClosed
	}
	if m.started {
		m.lifecycleMu.Unlock()
		return nil
	}
	m.started = true
	if m.retentionStore == nil || m.retention < 24*time.Hour {
		m.lifecycleMu.Unlock()
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	m.workerCancel = cancel
	m.workerDone = done
	store := m.retentionStore
	retention := m.retention
	logger := m.logger
	m.lifecycleMu.Unlock()

	go func() {
		defer close(done)
		runQuoteRetentionWorker(workerCtx, store, retention, logger)
	}()
	return nil
}

// Wait 等待当前 Module Worker 退出；不会主动取消 Worker。
func (m *Module) Wait(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	m.lifecycleMu.Lock()
	done := m.workerDone
	m.lifecycleMu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop 取消并等待 Module Worker 退出；重复调用安全。
func (m *Module) Stop(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	m.lifecycleMu.Lock()
	cancel := m.workerCancel
	done := m.workerDone
	if cancel != nil {
		cancel()
	}
	m.lifecycleMu.Unlock()
	if done == nil {
		m.lifecycleMu.Lock()
		if !m.closed && !m.closing {
			m.started = false
		}
		m.lifecycleMu.Unlock()
		return nil
	}
	select {
	case <-done:
		m.lifecycleMu.Lock()
		if m.workerDone == done {
			m.workerCancel = nil
			m.workerDone = nil
			if !m.closed && !m.closing {
				m.started = false
			}
		}
		m.lifecycleMu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close 停止并等待 Worker 后关闭 Module 自有资源；不会关闭调用方注入的 Store/Cache。
func (m *Module) Close(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	m.closeMu.Lock()
	defer m.closeMu.Unlock()

	m.lifecycleMu.Lock()
	if m.closed {
		err := m.closeErr
		m.lifecycleMu.Unlock()
		return err
	}
	m.closing = true
	cancel := m.workerCancel
	done := m.workerDone
	if cancel != nil {
		cancel()
	}
	m.lifecycleMu.Unlock()

	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			m.lifecycleMu.Lock()
			m.closing = false
			m.lifecycleMu.Unlock()
			return ctx.Err()
		}
	}

	m.lifecycleMu.Lock()
	m.closed = true
	m.closing = false
	m.started = false
	m.workerCancel = nil
	m.workerDone = nil
	client := m.baostockClient
	m.lifecycleMu.Unlock()

	var closeErr error
	if client != nil {
		closeErr = client.Close()
	}
	m.lifecycleMu.Lock()
	m.closeErr = closeErr
	m.lifecycleMu.Unlock()
	return closeErr
}
