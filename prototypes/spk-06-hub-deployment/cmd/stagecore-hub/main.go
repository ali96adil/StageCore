package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-06-hub-deployment/internal/config"
	"github.com/ali96adil/StageCore/prototypes/spk-06-hub-deployment/internal/server"
	"github.com/ali96adil/StageCore/prototypes/spk-06-hub-deployment/internal/storage"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(2)
	}
	layout := storage.Layout{DataRoot: cfg.DataRoot, VaultRoot: cfg.VaultRoot}
	if err := layout.Ensure(); err != nil {
		slog.Error("storage layout", "error", err)
		os.Exit(3)
	}
	hubID, err := layout.HubID()
	if err != nil {
		slog.Error("hub identity", "error", err)
		os.Exit(4)
	}

	app := server.New(cfg, layout, hubID)
	httpServer := &http.Server{Addr: cfg.BindAddress, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("stagecore hub listening", "addr", cfg.BindAddress, "hub_id", hubID, "data_root", cfg.DataRoot, "vault_root", cfg.VaultRoot)
		errCh <- httpServer.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		slog.Info("shutdown requested", "signal", sig.String())
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server stopped", "error", err)
			os.Exit(5)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown", "error", err)
		os.Exit(6)
	}
}
