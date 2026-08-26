package app

import (
	"context"
	"fmt"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/store"
)

type App struct {
	Config    config.Config
	DB        *db.Handle
	Store     *store.Store
	CueEngine *cueengine.Engine
	OSCPlugin *pluginhost.Host
}

func Open(ctx context.Context, cfg config.Config) (*App, error) {
	handle, err := db.Open(ctx, db.Config{DataRoot: cfg.DataRoot})
	if err != nil {
		return nil, fmt.Errorf("open StageCore database: %w", err)
	}

	s := store.New(handle.DB, clock.Real{})
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

	return &App{
		Config:    cfg,
		DB:        handle,
		Store:     s,
		CueEngine: cueengine.NewWithExecutor(s, registry),
		OSCPlugin: oscHost,
	}, nil
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	if a.OSCPlugin != nil {
		a.OSCPlugin.Close()
	}
	if a.DB == nil {
		return nil
	}
	return a.DB.Close()
}
