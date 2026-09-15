package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/dispatchauthority"
	"github.com/ali96adil/StageCore/internal/haauthority"
	"github.com/ali96adil/StageCore/internal/hawitness"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
)

// HAAuthority is the product-facing optional HA control surface. Merely
// constructing it never acquires a witness lease. Explicit Acquire/Renew/
// Release control remains a later operator/supervisor slice.
type HAAuthority interface {
	dispatchauthority.Source
	Acquire(context.Context) error
	Renew(context.Context) error
	Release(context.Context) error
	Demote()
}

func physicalDispatchForHA(ctx context.Context, cfg config.Config, registry *capability.Registry, hubSecurity *hubsecurity.Service) (capability.Executor, HAAuthority, error) {
	if registry == nil {
		return nil, nil, fmt.Errorf("physical capability registry is required")
	}
	mode := strings.ToUpper(strings.TrimSpace(cfg.HAMode))
	if mode == "" || mode == config.HAModeStandalone {
		if strings.TrimSpace(cfg.HAWitnessURL) != "" || strings.TrimSpace(cfg.HAWitnessID) != "" || strings.TrimSpace(cfg.HAWitnessFingerprint) != "" {
			return nil, nil, fmt.Errorf("HA witness settings require HA mode %s", config.HAModeWitness)
		}
		return dispatchauthority.NewStandalone(registry), nil, nil
	}
	if mode != config.HAModeWitness {
		return nil, nil, fmt.Errorf("unsupported Hub HA mode %q", cfg.HAMode)
	}
	if strings.TrimSpace(cfg.HAWitnessURL) == "" || strings.TrimSpace(cfg.HAWitnessID) == "" || strings.TrimSpace(cfg.HAWitnessFingerprint) == "" {
		return nil, nil, fmt.Errorf("HA witness URL, ID, and fingerprint are required in %s mode", config.HAModeWitness)
	}
	if hubSecurity == nil {
		return nil, nil, fmt.Errorf("Hub security identity is required for HA witness mode")
	}
	certificate, err := hubSecurity.HATransportCertificate(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("derive Hub HA transport identity: %w", err)
	}
	client, err := hawitness.NewClient(hawitness.ClientConfig{
		BaseURL:            cfg.HAWitnessURL,
		HubCertificate:     certificate,
		WitnessID:          cfg.HAWitnessID,
		WitnessFingerprint: cfg.HAWitnessFingerprint,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("configure HA witness client: %w", err)
	}
	controller, err := haauthority.New(client)
	if err != nil {
		return nil, nil, fmt.Errorf("configure HA dispatch authority: %w", err)
	}
	return dispatchauthority.New(registry, controller), controller, nil
}
