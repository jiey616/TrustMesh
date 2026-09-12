package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
	"trustmesh/backend/internal/app"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/logger"
)

func main() {
	_ = godotenv.Load(".env", "backend/.env")
	cfg := config.Load()

	log, err := logger.New(cfg.LogLevel)
	if err != nil {
		panic(fmt.Sprintf("failed to init logger: %v", err))
	}
	defer func() { _ = log.Sync() }()

	application, err := app.New(cfg, log)
	if err != nil {
		log.Fatal("failed to initialize app", zap.Error(err))
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			log.Error("failed to close app resources", zap.Error(closeErr))
		}
	}()

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      application.Engine,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	go func() {
		log.Info("server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server failed", zap.Error(err))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", zap.Error(err))
	}

	// 有界落盘：HTTP 流量已 drain、Mongo 尚未断开时，把全内存状态尽力回写一次。
	// 使用独立于 ShutdownGrace 的预算（默认 30s），因为 Mongo 抖动的回写可能比
	// HTTP drain 慢得多。ctx 到期即放弃（fail-fast），绝不无限挂起。
	flushCtx, flushCancel := context.WithTimeout(context.Background(), cfg.ShutdownFlushTimeout)
	defer flushCancel()
	if err := application.FlushAll(flushCtx); err != nil {
		log.Error("bounded shutdown flush failed; latest in-memory state may not be persisted", zap.Error(err))
	} else {
		log.Info("shutdown flush complete")
	}

	log.Info("server stopped")
}
