package app

import (
	"context"
	"fmt"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/store"
)

type App struct {
	Config config.Config
	DB     *db.Handle
	Store  *store.Store
}

func Open(ctx context.Context, cfg config.Config) (*App, error) {
	handle, err := db.Open(ctx, db.Config{DataRoot: cfg.DataRoot})
	if err != nil {
		return nil, fmt.Errorf("open StageCore database: %w", err)
	}
	return &App{Config: cfg, DB: handle, Store: store.New(handle.DB, clock.Real{})}, nil
}

func (a *App) Close() error {
	if a == nil || a.DB == nil {
		return nil
	}
	return a.DB.Close()
}
