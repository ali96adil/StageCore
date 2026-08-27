package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/ali96adil/StageCore/prototypes/spk-01-core-stack/internal/core"
	"github.com/ali96adil/StageCore/prototypes/spk-01-core-stack/internal/httpapi"
	"github.com/ali96adil/StageCore/prototypes/spk-01-core-stack/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	data := flag.String("data", "./var/spike-state.json", "prototype state file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	bus := core.NewBus()
	svc, err := core.NewService(store.File{Path: *data}, bus)
	if err != nil { logger.Error("open state", "error", err); os.Exit(1) }
	server := &http.Server{Addr: *addr, Handler: httpapi.New(svc, bus)}
	logger.Info("StageCore SPK-01 listening", "addr", *addr, "data", *data)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed { logger.Error("server stopped", "error", err); os.Exit(1) }
}
