package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ali96adil/StageCore/internal/app"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/httpapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.Open(ctx, cfg)
	if err != nil {
		logger.Error("hub startup failed", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	api := httpapi.New(
		httpapi.WithCompanionAuth(application.CompanionAuth),
		httpapi.WithCompanionRuntime(application.CompanionRuntime),
		httpapi.WithVault(application.Vault),
		httpapi.WithSoftwareRepository(application.Software),
	)
	server := &http.Server{Addr: cfg.Listen, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("StageCore Hub listening", "listen", cfg.Listen, "data_root", cfg.DataRoot)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP shutdown failed", "error", err)
	}
}
