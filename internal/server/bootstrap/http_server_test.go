package bootstrap

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

type testDrainingReadiness struct {
	draining atomic.Bool
}

func (r *testDrainingReadiness) MarkDraining() { r.draining.Store(true) }

func TestHTTPServerComponentStopsReadinessAndWaitsForInflightRequest(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	readiness := &testDrainingReadiness{}
	component, err := newHTTPServerComponent(&http.Server{Handler: mux}, "127.0.0.1:0", readiness)
	if err != nil {
		t.Fatal(err)
	}
	if err := component.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	responseDone := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + component.listener.Addr().String() + "/slow")
		if response != nil {
			_ = response.Body.Close()
		}
		responseDone <- err
	}()
	<-entered

	stopDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		stopDone <- component.Stop(ctx)
	}()
	deadline := time.After(time.Second)
	for !readiness.draining.Load() {
		select {
		case <-deadline:
			t.Fatal("readiness was not marked draining")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	select {
	case err := <-stopDone:
		t.Fatalf("Stop returned before request completed: %v", err)
	default:
	}
	close(release)
	if err := <-responseDone; err != nil {
		t.Fatalf("in-flight request failed: %v", err)
	}
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := component.Result(); err != nil {
		t.Fatalf("Result() error = %v", err)
	}
}

func TestHTTPServerComponentListenFailureIsStartupFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	component, err := newHTTPServerComponent(&http.Server{Handler: http.NewServeMux()}, listener.Addr().String(), &testDrainingReadiness{})
	if err != nil {
		t.Fatal(err)
	}
	if err := component.Start(context.Background()); err == nil {
		t.Fatal("Start() must return bind errors")
	}
}

func TestHTTPServerComponentShutdownTimeoutForcesClose(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/blocked", func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	})
	component, err := newHTTPServerComponent(&http.Server{Handler: mux}, "127.0.0.1:0", &testDrainingReadiness{})
	if err != nil {
		t.Fatal(err)
	}
	if err := component.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, _ := http.Get("http://" + component.listener.Addr().String() + "/blocked")
		if response != nil {
			_ = response.Body.Close()
		}
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = component.Stop(ctx)
	if err == nil {
		t.Fatal("Stop() must report shutdown timeout")
	}
	close(release)
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("request goroutine did not exit after forced close")
	}
}
