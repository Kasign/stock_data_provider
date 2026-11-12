package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"log/slog"
)

type drainingReadiness interface {
	MarkDraining()
}

// httpServerComponent 将 Listener 的创建、服务错误和优雅关闭纳入统一生命周期。
type httpServerComponent struct {
	server      *http.Server
	address     string
	readiness   drainingReadiness
	connections *activeConnectionTracker

	mu       sync.Mutex
	listener net.Listener
	started  bool
	serveErr error
	done     chan struct{}
}

func newHTTPServerComponent(server *http.Server, address string, readiness drainingReadiness) (*httpServerComponent, error) {
	if server == nil || address == "" || readiness == nil {
		return nil, errors.New("HTTP server, address and readiness handler are required")
	}
	connections := newActiveConnectionTracker()
	previousConnState := server.ConnState
	server.ConnState = func(conn net.Conn, state http.ConnState) {
		connections.observe(conn, state)
		if previousConnState != nil {
			previousConnState(conn, state)
		}
	}
	return &httpServerComponent{
		server: server, address: address, readiness: readiness,
		connections: connections, done: make(chan struct{}),
	}, nil
}

// activeConnectionTracker waits for net/http connection handlers to finish.
// Server.Close closes active sockets but does not wait for their handlers.
type activeConnectionTracker struct {
	mu     sync.Mutex
	active map[net.Conn]struct{}
	idle   chan struct{}
}

func newActiveConnectionTracker() *activeConnectionTracker {
	idle := make(chan struct{})
	close(idle)
	return &activeConnectionTracker{active: make(map[net.Conn]struct{}), idle: idle}
}

func (t *activeConnectionTracker) observe(conn net.Conn, state http.ConnState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch state {
	case http.StateActive:
		if _, exists := t.active[conn]; !exists {
			if len(t.active) == 0 {
				t.idle = make(chan struct{})
			}
			t.active[conn] = struct{}{}
		}
	case http.StateIdle, http.StateClosed, http.StateHijacked:
		if _, exists := t.active[conn]; exists {
			delete(t.active, conn)
			if len(t.active) == 0 {
				close(t.idle)
			}
		}
	}
}

func (t *activeConnectionTracker) wait(ctx context.Context) error {
	if ctx == nil {
		return errors.New("connection drain context is required")
	}
	t.mu.Lock()
	idle := t.idle
	t.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *httpServerComponent) Name() string { return "http-server" }

func (c *httpServerComponent) Start(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return errors.New("HTTP server component already started")
	}
	listener, err := net.Listen("tcp", c.address)
	if err != nil {
		return fmt.Errorf("listen HTTP address %q: %w", c.address, err)
	}
	c.listener = listener
	c.started = true
	go func() {
		err := c.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		c.mu.Lock()
		c.serveErr = err
		c.mu.Unlock()
		close(c.done)
	}()
	slog.Info("Root Server HTTP 已启动", "addr", listener.Addr().String())
	return nil
}

func (c *httpServerComponent) Done() <-chan struct{} { return c.done }

func (c *httpServerComponent) Result() error {
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.serveErr != nil {
		return fmt.Errorf("HTTP 服务异常退出: %w", c.serveErr)
	}
	return nil
}

func (c *httpServerComponent) Stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("HTTP server shutdown context is required")
	}
	c.mu.Lock()
	started := c.started
	c.mu.Unlock()
	if !started {
		return nil
	}
	c.readiness.MarkDraining()
	shutdownErr := c.server.Shutdown(ctx)
	if shutdownErr != nil {
		closeErr := c.server.Close()
		if closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			shutdownErr = errors.Join(shutdownErr, closeErr)
		}
	}
	// Serve 退出只代表 Listener 已关闭；Close 不等待已进入的 Handler。
	// 必须等活动连接进入 Idle/Closed 后才能让 Bootstrap 释放共享 DB/Redis。
	<-c.done
	if err := c.connections.wait(ctx); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("wait for active HTTP handlers: %w", err))
	}
	if serveErr := c.Result(); serveErr != nil {
		shutdownErr = errors.Join(shutdownErr, serveErr)
	}
	return shutdownErr
}
