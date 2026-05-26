package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		panic(err)
	}

	app, err := NewApp(cfg)
	if err != nil {
		appLogger := slog.Default()
		appLogger.Error("failed to initialize service", "error", err)
		panic(err)
	}

	handler, staticDir, err := app.routes()
	if err != nil {
		app.log.Error("failed to configure routes", "error", err)
		panic(err)
	}

	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}

	app.log.Info("service starting",
		"addr", cfg.Addr(),
		"static_dir", staticDir,
		"auth_enabled", app.authEnabled(),
		"users_file", cfg.UsersFile,
		"data_dir", cfg.DataDir,
		"recovered_from", app.recoveredFrom,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.persistence.SaveNow(shutdownCtx, "shutdown"); err != nil {
			app.log.Error("shutdown snapshot failed", "error", err)
		}
		if err := server.Shutdown(shutdownCtx); err != nil {
			app.log.Error("http shutdown failed", "error", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		app.log.Error("server failed", "error", err)
		panic(err)
	}
}
