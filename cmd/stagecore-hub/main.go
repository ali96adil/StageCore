package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ali96adil/StageCore/internal/app"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/deviceprofile"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/securitypreflight"
	"github.com/ali96adil/StageCore/internal/sessionmemory"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type oscInputRuntime interface {
	StartOSCInputForProject(context.Context, string, string) (string, error)
	ServeOSCInput(context.Context) error
}

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
		application.HubSecurity,
		application.SecretStore,
		application.PluginPermissions,
	)
	runtime := runtimecontrol.New(
		application.Store,
		application.Capabilities,
		runtimecontrol.WithShowGate(preflightService.ShowGate),
	)
	memory := sessionmemory.New(application.Store)
	deviceProfiles := deviceprofile.BuiltinCatalog()
	extensionLibrary, err := extension.NewLibrary(application.Store, application.Software)
	if err != nil {
		logger.Error("extension library startup failed", "error", err)
		os.Exit(1)
	}
	extensionInstaller, err := extension.NewInstaller(
		extensionLibrary,
		filepath.Join(application.Config.DataRoot, "extensions"),
		extension.WithInstallerCapacityPolicy(storagehealth.NewPolicy(application.Config.RuntimeReserveBytes, application.Config.StorageWarningPercent)),
	)
	if err != nil {
		logger.Error("extension installer startup failed", "error", err)
		os.Exit(1)
	}
	extensionPermissionReviewer, err := extension.NewPermissionReviewer(extensionInstaller)
	if err != nil {
		logger.Error("extension permission review startup failed", "error", err)
		os.Exit(1)
	}

	api := httpapi.New(
		httpapi.WithOperatorWeb(),
		httpapi.WithFirstOwnerBootstrap(application.HubSecurity, application.SecurityAudit),
		httpapi.WithUserAuth(userAuth, application.HubSecurity, application.SecurityAudit),
		httpapi.WithOperatorProjects(userAuth, application.Store),
		httpapi.WithOperatorDeviceProfiles(userAuth, deviceProfiles),
		httpapi.WithOperatorExtensionLibrary(userAuth, extensionLibrary, application.SecurityAudit),
		httpapi.WithOperatorExtensionInstaller(userAuth, extensionInstaller, application.SecurityAudit),
		httpapi.WithOperatorExtensionPermissionReview(userAuth, extensionPermissionReviewer, application.SecurityAudit),
		httpapi.WithOperatorMachineRoles(userAuth, application.Store),
		httpapi.WithOperatorConfiguration(userAuth, application.Store),
		httpapi.WithOperatorConfigurationDraft(userAuth, application.Store),
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
		Addr:              cfg.Listen,
		Handler:           httpapi.AuditDeniedRequests(api.Handler(), userAuth, application.SecurityAudit),
		ReadHeaderTimeout: 5 * time.Second,
	}

	httpErrCh := make(chan error, 1)
	deviceErrCh := make(chan error, 1)
	oscInputErrCh := make(chan error, 1)
	gateway, err := startDeviceGateway(ctx, logger, application, cfg.DeviceListen, deviceErrCh)
	if err != nil {
		logger.Error("secure device gateway startup failed", "error", err)
		os.Exit(1)
	}
	if cfg.OSCInputListen != "" {
		listen, err := startOSCInput(ctx, application, cfg.OSCInputProjectID, cfg.OSCInputListen, oscInputErrCh)
		if err != nil {
			logger.Error("OSC input startup failed", "error", err)
			os.Exit(1)
		}
		logger.Info("StageCore OSC input listening", "listen", listen, "project_id", cfg.OSCInputProjectID)
	}
	go func() {
		logger.Info("StageCore Hub listening", "listen", cfg.Listen, "data_root", cfg.DataRoot)
		httpErrCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-httpErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
		}
	case err := <-deviceErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("secure device gateway failed", "error", err)
		}
	case err := <-oscInputErrCh:
		if err != nil {
			logger.Error("OSC input failed", "error", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := gateway.Shutdown(shutdownCtx); err != nil {
		logger.Error("secure device gateway shutdown failed", "error", err)
	}
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP shutdown failed", "error", err)
	}
}

func startOSCInput(ctx context.Context, runtime oscInputRuntime, projectID, listenAddress string, errCh chan<- error) (string, error) {
	listen, err := runtime.StartOSCInputForProject(ctx, projectID, listenAddress)
	if err != nil {
		return "", err
	}
	go func() {
		errCh <- runtime.ServeOSCInput(ctx)
	}()
	return listen, nil
}
