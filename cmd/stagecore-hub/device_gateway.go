package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/ali96adil/StageCore/internal/app"
	"github.com/ali96adil/StageCore/internal/discovery"
	"github.com/ali96adil/StageCore/internal/httpapi"
)

type deviceGateway struct {
	server     *http.Server
	listener   net.Listener
	advertiser *discovery.Advertiser
}

func startDeviceGateway(
	ctx context.Context,
	logger *slog.Logger,
	application *app.App,
	listenAddress string,
	errCh chan<- error,
) (*deviceGateway, error) {
	if application == nil || application.HubSecurity == nil || application.CompanionAuth == nil || application.CompanionRuntime == nil {
		return nil, fmt.Errorf("device gateway requires Hub identity and Companion services")
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
	} else if advertiser, advertiseErr := discovery.Start(ctx, announcement); advertiseErr != nil {
		logger.Warn("StageCore Bonjour discovery unavailable", "error", advertiseErr)
	} else {
		gateway.advertiser = advertiser
		logger.Info(
			"StageCore Hub discovery active",
			"service", discovery.ServiceType,
			"hub_id", identity.HubID,
			"device_listen", listenAddress,
		)
	}

	go func() {
		logger.Info("StageCore secure device gateway listening", "listen", listenAddress, "hub_id", identity.HubID)
		errCh <- server.Serve(tlsListener)
	}()
	return gateway, nil
}

func (g *deviceGateway) Shutdown(ctx context.Context) error {
	if g == nil {
		return nil
	}
	if g.advertiser != nil {
		_ = g.advertiser.Close()
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
