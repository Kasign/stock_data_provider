package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/pythonapi"
	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
	"example.com/stock-analysis-flow/internal/platform/config"
	"example.com/stock-analysis-flow/internal/platform/httpapi"
	"example.com/stock-analysis-flow/internal/service"
)

func main() {
	if err := config.LoadDotEnvIfExists(".env"); err != nil {
		log.Fatalf("load .env: %v", err)
	}
	storageCfg, err := mysqlredis.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("storage configuration: %v", err)
	}
	pythonCfg, err := pythonapi.ConfigFromEnv()
	if err != nil {
		log.Fatalf("Python API configuration: %v", err)
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 15*time.Second)
	store, err := mysqlredis.Open(startupCtx, storageCfg)
	startupCancel()
	if err != nil {
		log.Fatalf("open MariaDB/Redis (check DB_HOST/DB_PASSWORD/REDIS_ADDR and DB_AUTO_MIGRATE): %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("WARN: close dependencies: %v", err)
		}
	}()

	runner, err := service.NewRunnerWithStores(pythonCfg, store, store)
	if err != nil {
		log.Fatalf("build analysis runner: %v", err)
	}
	analysisTimeout, err := service.EnvDuration("ANALYSIS_TIMEOUT", 3*time.Minute)
	if err != nil {
		log.Fatal(err)
	}
	api, err := httpapi.New(runner, store, analysisTimeout)
	if err != nil {
		log.Fatalf("build HTTP API: %v", err)
	}
	addr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if addr == "" {
		addr = ":8092"
	}
	readTimeout, err := service.EnvDuration("HTTP_READ_TIMEOUT", 15*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	writeTimeout, err := service.EnvDuration("HTTP_WRITE_TIMEOUT", analysisTimeout+10*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	idleTimeout, err := service.EnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: addr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: readTimeout, WriteTimeout: writeTimeout, IdleTimeout: idleTimeout}

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("stock-analysis Go API listening on %s; Python API=%s; DB_AUTO_MIGRATE=%t", addr, pythonCfg.BaseURL, storageCfg.DBAutoMigrate)
		serveErr <- server.ListenAndServe()
	}()
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server: %v", err)
		}
	case <-signalCtx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("WARN: graceful HTTP shutdown: %v", err)
		}
		if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) && err != nil {
			log.Printf("WARN: HTTP server stopped: %v", err)
		}
	}
}
