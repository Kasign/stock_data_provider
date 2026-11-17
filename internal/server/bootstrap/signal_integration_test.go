package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	serverruntime "example.com/stock-analysis-flow/internal/server/runtime"
)

const sigtermHelperEnv = "SERVER_SIGTERM_HELPER"

type signalTestWorker struct {
	eventFile string
}

func (w signalTestWorker) Name() string                { return "test-worker" }
func (w signalTestWorker) Start(context.Context) error { return nil }
func (w signalTestWorker) Stop(context.Context) error {
	return appendSignalEvent(w.eventFile, "worker-stopped")
}

type signalTestReadiness struct {
	eventFile string
}

func (r signalTestReadiness) MarkDraining() {
	_ = appendSignalEvent(r.eventFile, "http-draining")
}

func appendSignalEvent(path, event string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, event)
	return err
}

// TestSIGTERMHelperProcess 在子进程中执行真实 OS SIGTERM 通知与 Lifecycle 停止链路。
func TestSIGTERMHelperProcess(t *testing.T) {
	if os.Getenv(sigtermHelperEnv) != "1" {
		return
	}
	eventFile := os.Getenv("SERVER_SIGTERM_EVENT_FILE")
	readyFile := os.Getenv("SERVER_SIGTERM_READY_FILE")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	lifecycle := serverruntime.NewLifecycle()
	if err := lifecycle.Register(signalTestWorker{eventFile: eventFile}); err != nil {
		t.Fatal(err)
	}
	server, err := newHTTPServerComponent(
		&http.Server{Handler: http.NewServeMux()},
		"127.0.0.1:0",
		signalTestReadiness{eventFile: eventFile},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Register(server); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readyFile, []byte(server.listener.Addr().String()), 0o600); err != nil {
		t.Fatal(err)
	}
	<-ctx.Done()
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := lifecycle.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}

// TestSIGTERMStopsHTTPBeforeWorker 使用独立测试子进程接收真实 SIGTERM，验证关闭顺序。
func TestSIGTERMStopsHTTPBeforeWorker(t *testing.T) {
	if os.Getenv(sigtermHelperEnv) == "1" {
		t.Skip("helper process is exercised by the parent test")
	}
	dir := t.TempDir()
	eventFile := filepath.Join(dir, "events.log")
	readyFile := filepath.Join(dir, "ready")
	cmd := exec.Command(os.Args[0], "-test.run=^TestSIGTERMHelperProcess$")
	cmd.Env = append(os.Environ(),
		sigtermHelperEnv+"=1",
		"SERVER_SIGTERM_EVENT_FILE="+eventFile,
		"SERVER_SIGTERM_READY_FILE="+readyFile,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}
	waitDone := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(waitDone)
	}()
	t.Cleanup(func() {
		select {
		case <-waitDone:
		default:
			_ = cmd.Process.Kill()
			<-waitDone
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(readyFile); err == nil {
			break
		}
		select {
		case <-waitDone:
			t.Fatalf("helper exited before ready: %v", waitErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
	select {
	case <-waitDone:
		if waitErr != nil {
			t.Fatalf("helper process failed: %v", waitErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper process did not exit after SIGTERM")
	}
	raw, err := os.ReadFile(eventFile)
	if err != nil {
		t.Fatalf("read shutdown events: %v", err)
	}
	events := strings.Fields(string(raw))
	want := []string{"http-draining", "worker-stopped"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("shutdown events = %v, want %v", events, want)
	}
}
