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
	server    *http.Server
	address   string
	readiness drainingReadiness

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
	return &httpServerComponent{server: server, address: address, readiness: readiness, done: make(chan struct{})}, nil
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
	select {
	case <-c.done:
	case <-ctx.Done():
		_ = c.server.Close()
		<-c.done
		shutdownErr = errors.Join(shutdownErr, ctx.Err())
	}
	if serveErr := c.Result(); serveErr != nil {
		shutdownErr = errors.Join(shutdownErr, serveErr)
	}
	return shutdownErr
}
