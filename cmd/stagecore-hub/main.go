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
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/securitypreflight"
	"github.com/ali96adil/StageCore/internal/sessionmemory"
	"github.com/ali96adil/StageCore/internal/userauth"
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

	userAuth, err := userauth.New(application.DB.DB)
	if err != nil {
		logger.Error("browser authentication startup failed", "error", err)
		os.Exit(1)
	}
	publisher := publish.New(application.Store, application.Capabilities)
	basePreflight := preflight.New(
		application.Store,
		application.Capabilities,
		application.StorageHealth,
		preflight.WithConnectionCheck(application.CompanionRuntime.IsConnected),
	)
	preflightService := securitypreflight.New(
		basePreflight,
		application.Store,
		application.SecretStore,
		application.PluginPermissions,
	)
	runtime := runtimecontrol.New(
		application.Store,
		application.Capabilities,
		runtimecontrol.WithShowGate(preflightService.ShowGate),
	)
	memory := sessionmemory.New(application.Store)

	api := httpapi.New(
		httpapi.WithOperatorWeb(),
		httpapi.WithUserAuth(userAuth, application.HubSecurity, application.SecurityAudit),
		httpapi.WithOperatorProjects(userAuth, application.Store),
		httpapi.WithOperatorCuePublish(userAuth, application.Store, publisher),
		httpapi.WithOperatorPreflight(userAuth, preflightService),
		httpapi.WithOperatorRuntime(userAuth, application.Store, runtime),
		httpapi.WithOperatorMemory(userAuth, application.Store, memory),
		httpapi.WithSecurityOperations(
			userAuth,
			application.Store,
			application.SecretStore,
			application.PluginPermissions,
			application.SecurityAudit,
			application.CompanionAuth,
			application.RefreshPluginPermissions,
		),
		httpapi.WithCompanionAuth(application.CompanionAuth),
		httpapi.WithCompanionRuntime(application.CompanionRuntime),
		httpapi.WithVault(application.Vault),
		httpapi.WithSoftwareRepository(application.Software),
		httpapi.WithBulkManager(application.Bulk),
		httpapi.WithStorageHealth(application.StorageHealth),
	)
	server := &http.Server{
		Addr: cfg.Listen,
		Handler: httpapi.AuditDeniedRequests(api.Handler(), userAuth, application.SecurityAudit),
		ReadHeaderTimeout: 5 * time.Second,
	}

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
