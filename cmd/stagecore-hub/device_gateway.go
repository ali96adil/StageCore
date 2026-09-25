package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/app"
	"github.com/ali96adil/StageCore/internal/discovery"
	"github.com/ali96adil/StageCore/internal/httpapi"
)

const bonjourRetryInterval = 5 * time.Second

type discoveryStarter func(context.Context, discovery.Announcement) (*discovery.Advertiser, error)

type deviceGateway struct {
	server   *http.Server
	listener net.Listener

	advertiserMu sync.Mutex
	advertiser   *discovery.Advertiser
	closed       bool
}

func startDeviceGateway(
	ctx context.Context,
	logger *slog.Logger,
	application *app.App,
	listenAddress string,
	errCh chan<- error,
) (*deviceGateway, error) {
	if application == nil || application.HubSecurity == nil || application.CompanionAuth == nil || application.CompanionRuntime == nil || application.DeviceExperience == nil || application.DeviceRuntime == nil {
		return nil, fmt.Errorf("device gateway requires Hub identity, Companion services and Stage Device runtime")
	}
	certificate, certificatePin, err := application.HubSecurity.DeviceTLSCertificate(ctx)
	if err != nil {
		return nil, fmt.Errorf("prepare Hub device TLS identity: %w", err)
	}
	identity, err := application.HubSecurity.Identity(ctx)
	if err != nil {
		return nil, fmt.Errorf("read Hub identity for discovery: %w", err)
	}

	deviceAPI := httpapi.New(
		httpapi.WithHubIdentity(application.HubSecurity),
		httpapi.WithCompanionAuth(application.CompanionAuth),
		httpapi.WithCompanionRuntime(application.CompanionRuntime),
		httpapi.WithStageDeviceRuntime(application.CompanionAuth, application.DeviceRuntime),
		httpapi.WithVault(application.Vault),
	)
	server := &http.Server{
		Addr:              listenAddress,
		Handler:           deviceAPI.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{certificate},
		},
	}
	plainListener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen for secure StageCore devices on %s: %w", listenAddress, err)
	}
	tlsListener := tls.NewListener(plainListener, server.TLSConfig)
	gateway := &deviceGateway{server: server, listener: tlsListener}

	announcement, announceErr := discovery.NewAnnouncement(
		identity.HubID,
		identity.DisplayName,
		identity.Fingerprint,
		certificatePin,
		listenAddress,
	)
	if announceErr != nil {
		logger.Warn("StageCore Bonjour discovery unavailable", "error", announceErr)
	} else {
		startBonjourDiscovery(ctx, logger, gateway, announcement, identity.HubID, listenAddress)
	}

	go func() {
		logger.Info("StageCore secure device gateway listening", "listen", listenAddress, "hub_id", identity.HubID)
		errCh <- server.Serve(tlsListener)
	}()
	return gateway, nil
}

func startBonjourDiscovery(
	ctx context.Context,
	logger *slog.Logger,
	gateway *deviceGateway,
	announcement discovery.Announcement,
	hubID string,
	listenAddress string,
) {
	advertiser, err := discovery.Start(ctx, announcement)
	if err == nil {
		if gateway.installAdvertiser(advertiser) {
			logDiscoveryActive(logger, hubID, listenAddress, false)
		} else {
			_ = advertiser.Close()
		}
		return
	}

	logger.Warn(
		"StageCore Bonjour discovery unavailable",
		"error", err,
		"retrying", true,
		"retry_interval", bonjourRetryInterval.String(),
	)
	go func() {
		ticker := time.NewTicker(bonjourRetryInterval)
		defer ticker.Stop()
		retryBonjourDiscovery(
			ctx,
			logger,
			gateway,
			announcement,
			hubID,
			listenAddress,
			ticker.C,
			discovery.Start,
		)
	}()
}

func retryBonjourDiscovery(
	ctx context.Context,
	logger *slog.Logger,
	gateway *deviceGateway,
	announcement discovery.Announcement,
	hubID string,
	listenAddress string,
	retry <-chan time.Time,
	start discoveryStarter,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-retry:
			advertiser, err := start(ctx, announcement)
			if err != nil {
				continue
			}
			if !gateway.installAdvertiser(advertiser) {
				_ = advertiser.Close()
				return
			}
			logDiscoveryActive(logger, hubID, listenAddress, true)
			return
		}
	}
}

func logDiscoveryActive(logger *slog.Logger, hubID, listenAddress string, recovered bool) {
	logger.Info(
		"StageCore Hub discovery active",
		"service", discovery.ServiceType,
		"hub_id", hubID,
		"device_listen", listenAddress,
		"recovered", recovered,
	)
}

func (g *deviceGateway) installAdvertiser(advertiser *discovery.Advertiser) bool {
	if g == nil || advertiser == nil {
		return false
	}
	g.advertiserMu.Lock()
	defer g.advertiserMu.Unlock()
	if g.closed || g.advertiser != nil {
		return false
	}
	g.advertiser = advertiser
	return true
}

func (g *deviceGateway) takeAdvertiserForShutdown() *discovery.Advertiser {
	if g == nil {
		return nil
	}
	g.advertiserMu.Lock()
	defer g.advertiserMu.Unlock()
	g.closed = true
	advertiser := g.advertiser
	g.advertiser = nil
	return advertiser
}

func (g *deviceGateway) Shutdown(ctx context.Context) error {
	if g == nil {
		return nil
	}
	if advertiser := g.takeAdvertiserForShutdown(); advertiser != nil {
		_ = advertiser.Close()
	}
	if g.server == nil {
		return nil
	}
	err := g.server.Shutdown(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
