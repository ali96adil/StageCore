package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/oscinputplugin"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/store"
)

type App struct {
	Config           config.Config
	DB               *db.Handle
	Store            *store.Store
	CompanionAuth    *companionauth.Service
	CompanionRuntime *companionchannel.RuntimeChannel
	CueEngine        *cueengine.Engine
	RoutingEngine    *routing.Engine
	OSCPlugin        *pluginhost.Host
	OSCInput         *oscinputplugin.Host
}

func Open(ctx context.Context, cfg config.Config) (*App, error) {
	handle, err := db.Open(ctx, db.Config{DataRoot: cfg.DataRoot})
	if err != nil {
		return nil, fmt.Errorf("open StageCore database: %w", err)
	}

	s := store.New(handle.DB, clock.Real{})
	companionAuth := companionauth.New(s, nil)
	companionRuntime := companionchannel.NewRuntime(s, companionAuth)
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("register simulator capability: %w", err)
	}

	oscHost := pluginhost.New(
		cfg.OSCPluginPath,
		nil,
		nil,
		nil,
		pluginhost.Manifest{
			PluginID: oscplugin.PluginID,
			CapabilityPermissions: map[string][]string{
				oscplugin.CapabilityOSCSend: {oscplugin.PermissionUDPSend},
			},
			GrantedPermissions: []string{oscplugin.PermissionUDPSend},
		},
	)
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(oscHost)); err != nil {
		oscHost.Close()
		_ = handle.Close()
		return nil, fmt.Errorf("register OSC capability: %w", err)
	}
	if err := registry.RegisterTargetType(
		companion.MachineRoleLogicalType,
		companion.NewForwarder(s, companionRuntime, 5*time.Second, nil),
	); err != nil {
		companionRuntime.Close()
		oscHost.Close()
		_ = handle.Close()
		return nil, fmt.Errorf("register Companion target dispatch: %w", err)
	}

	return &App{
		Config:           cfg,
		DB:               handle,
		Store:            s,
		CompanionAuth:    companionAuth,
		CompanionRuntime: companionRuntime,
		CueEngine:        cueengine.NewWithExecutor(s, registry),
		RoutingEngine:    routing.New(s, registry),
		OSCPlugin:        oscHost,
	}, nil
}

// StartOSCInput starts the receive mode of the external stagecore.osc Plugin.
// M3 intentionally permits loopback listeners only; non-loopback Stage LAN
// input remains blocked until the SEC0-SEC2 authentication/transport gate.
func (a *App) StartOSCInput(ctx context.Context, sessionID, listenAddress string) (string, error) {
	if a == nil || a.RoutingEngine == nil {
		return "", fmt.Errorf("StageCore routing is unavailable")
	}
	if a.OSCInput != nil {
		a.OSCInput.Close()
		a.OSCInput = nil
	}
	host := oscinputplugin.New(
		a.Config.OSCPluginPath,
		listenAddress,
		nil,
		oscinputplugin.Manifest{
			PluginID: oscplugin.PluginID,
			InputPermissions: map[string][]string{
				oscplugin.InputOSCReceive: {oscplugin.PermissionUDPListen},
			},
			GrantedPermissions: []string{oscplugin.PermissionUDPListen},
		},
		a.RoutingEngine,
		sessionID,
	)
	if err := host.Start(ctx); err != nil {
		host.Close()
		return "", fmt.Errorf("start OSC input: %w", err)
	}
	a.OSCInput = host
	return host.LocalAddr(), nil
}

func (a *App) ServeOSCInput(ctx context.Context) error {
	if a == nil || a.OSCInput == nil {
		return fmt.Errorf("OSC input is not started")
	}
	return a.OSCInput.Serve(ctx)
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	if a.OSCInput != nil {
		a.OSCInput.Close()
	}
	if a.CompanionRuntime != nil {
		a.CompanionRuntime.Close()
	}
	if a.OSCPlugin != nil {
		a.OSCPlugin.Close()
	}
	if a.DB == nil {
		return nil
	}
	return a.DB.Close()
}
