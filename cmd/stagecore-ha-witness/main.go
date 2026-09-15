package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/halease"
	"github.com/ali96adil/StageCore/internal/hawitness"
)

const defaultListen = "0.0.0.0:7842"

type config struct {
	DataRoot       string
	Listen         string
	LeaseDuration  time.Duration
	AuthorizedHubs map[string]string
}

type repeatedFlag []string

func (values *repeatedFlag) String() string { return strings.Join(*values, ",") }

func (values *repeatedFlag) Set(value string) error {
	*values = append(*values, strings.TrimSpace(value))
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger, os.Args[1:]); err != nil {
		logger.Error("HA witness stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, args []string) error {
	cfg, err := loadConfig(args)
	if err != nil {
		return fmt.Errorf("configuration failed: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	identity, err := hawitness.LoadOrCreateIdentity(cfg.DataRoot, nil)
	if err != nil {
		return fmt.Errorf("open witness identity: %w", err)
	}
	authority, err := halease.Open(ctx, halease.Config{
		Path: filepath.Join(cfg.DataRoot, "db", "ha-witness.sqlite3"),
		LeaseDuration: cfg.LeaseDuration,
	}, clock.Real{})
	if err != nil {
		return fmt.Errorf("open witness lease authority: %w", err)
	}
	defer authority.Close()

	witness, err := hawitness.NewServer(authority, hawitness.ServerConfig{AuthorizedHubs: cfg.AuthorizedHubs})
	if err != nil {
		return fmt.Errorf("create witness server: %w", err)
	}
	tlsConfig, err := witness.TLSConfig(identity)
	if err != nil {
		return fmt.Errorf("configure witness TLS: %w", err)
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen for HA witness: %w", err)
	}
	defer listener.Close()
	tlsListener := tls.NewListener(listener, tlsConfig)

	httpServer := &http.Server{
		Handler: witness.Handler(),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout: 30 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		err := httpServer.Serve(tlsListener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	logger.Info(
		"StageCore HA witness listening",
		"listen", listener.Addr().String(),
		"witness_id", identity.ID,
		"fingerprint", identity.Fingerprint,
		"authorized_hubs", len(cfg.AuthorizedHubs),
		"lease_duration", cfg.LeaseDuration.String(),
	)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HA witness: %w", err)
		}
		if err := <-serveErr; err != nil {
			return fmt.Errorf("serve HA witness: %w", err)
		}
		return nil
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("serve HA witness: %w", err)
		}
		return nil
	}
}

func loadConfig(args []string) (config, error) {
	defaultDataRoot := envOr("STAGECORE_HA_WITNESS_DATA_ROOT", filepath.Join(".", "stagecore-ha-witness-data"))
	defaultListenAddress := envOr("STAGECORE_HA_WITNESS_LISTEN", defaultListen)
	defaultDurationText := envOr("STAGECORE_HA_WITNESS_LEASE_DURATION", halease.DefaultLeaseDuration.String())
	defaultDuration, err := time.ParseDuration(defaultDurationText)
	if err != nil {
		return config{}, fmt.Errorf("invalid STAGECORE_HA_WITNESS_LEASE_DURATION: %w", err)
	}

	var authorizedSpecs repeatedFlag
	if raw := strings.TrimSpace(os.Getenv("STAGECORE_HA_WITNESS_AUTHORIZED_HUBS")); raw != "" {
		for _, spec := range strings.Split(raw, ";") {
			if strings.TrimSpace(spec) != "" {
				authorizedSpecs = append(authorizedSpecs, strings.TrimSpace(spec))
			}
		}
	}

	fs := flag.NewFlagSet("stagecore-ha-witness", flag.ContinueOnError)
	dataRoot := fs.String("data-root", defaultDataRoot, "HA witness data root")
	listen := fs.String("listen", defaultListenAddress, "HA witness TLS listen address")
	leaseDuration := fs.Duration("lease-duration", defaultDuration, "witness-owned HA lease duration")
	fs.Var(&authorizedSpecs, "authorized-hub", "authorized Hub in hub_id=SHA256:fingerprint form; repeat for each Hub")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, errors.New("unexpected positional arguments")
	}

	cfg := config{
		DataRoot: strings.TrimSpace(*dataRoot),
		Listen: strings.TrimSpace(*listen),
		LeaseDuration: *leaseDuration,
		AuthorizedHubs: make(map[string]string),
	}
	if cfg.DataRoot == "" {
		return config{}, errors.New("HA witness data root is required")
	}
	if _, _, err := net.SplitHostPort(cfg.Listen); err != nil {
		return config{}, fmt.Errorf("invalid HA witness listen address %q: %w", cfg.Listen, err)
	}
	if cfg.LeaseDuration < halease.MinLeaseDuration || cfg.LeaseDuration > halease.MaxLeaseDuration {
		return config{}, fmt.Errorf("HA witness lease duration must be between %s and %s", halease.MinLeaseDuration, halease.MaxLeaseDuration)
	}
	for _, spec := range authorizedSpecs {
		hubID, fingerprint, ok := strings.Cut(spec, "=")
		hubID = strings.TrimSpace(hubID)
		fingerprint = strings.TrimSpace(fingerprint)
		if !ok || hubID == "" || fingerprint == "" {
			return config{}, fmt.Errorf("invalid authorized Hub %q; want hub_id=SHA256:fingerprint", spec)
		}
		if _, exists := cfg.AuthorizedHubs[hubID]; exists {
			return config{}, fmt.Errorf("duplicate authorized Hub ID %q", hubID)
		}
		cfg.AuthorizedHubs[hubID] = fingerprint
	}
	if len(cfg.AuthorizedHubs) == 0 {
		return config{}, errors.New("at least one --authorized-hub or STAGECORE_HA_WITNESS_AUTHORIZED_HUBS entry is required")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
