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

// HAAuthority is the low-level optional HA control surface kept for product
// inspection. Operator actions use HASupervisor so fresh authority acquisition
// always passes the Session guard and renewal lifecycle.
type HAAuthority interface {
	dispatchauthority.Source
	Acquire(context.Context) error
	Renew(context.Context) error
	Release(context.Context) error
	Demote()
}

func physicalDispatchForHA(
	ctx context.Context,
	cfg config.Config,
	registry *capability.Registry,
	hubSecurity *hubsecurity.Service,
	sessions haauthority.SessionReader,
) (capability.Executor, HAAuthority, *haauthority.Supervisor, error) {
	if registry == nil {
		return nil, nil, nil, fmt.Errorf("physical capability registry is required")
	}
	mode := strings.ToUpper(strings.TrimSpace(cfg.HAMode))
	if mode == "" || mode == config.HAModeStandalone {
		if strings.TrimSpace(cfg.HAWitnessURL) != "" || strings.TrimSpace(cfg.HAWitnessID) != "" || strings.TrimSpace(cfg.HAWitnessFingerprint) != "" {
			return nil, nil, nil, fmt.Errorf("HA witness settings require HA mode %s", config.HAModeWitness)
		}
		return dispatchauthority.NewStandalone(registry), nil, nil, nil
	}
	if mode != config.HAModeWitness {
		return nil, nil, nil, fmt.Errorf("unsupported Hub HA mode %q", cfg.HAMode)
	}
	if strings.TrimSpace(cfg.HAWitnessURL) == "" || strings.TrimSpace(cfg.HAWitnessID) == "" || strings.TrimSpace(cfg.HAWitnessFingerprint) == "" {
		return nil, nil, nil, fmt.Errorf("HA witness URL, ID, and fingerprint are required in %s mode", config.HAModeWitness)
	}
	if hubSecurity == nil {
		return nil, nil, nil, fmt.Errorf("Hub security identity is required for HA witness mode")
	}
	if sessions == nil {
		return nil, nil, nil, fmt.Errorf("HA Session authority is required for witness mode")
	}
	certificate, err := hubSecurity.HATransportCertificate(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("derive Hub HA transport identity: %w", err)
	}
	client, err := hawitness.NewClient(hawitness.ClientConfig{
		BaseURL:            cfg.HAWitnessURL,
		HubCertificate:     certificate,
		WitnessID:          cfg.HAWitnessID,
		WitnessFingerprint: cfg.HAWitnessFingerprint,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("configure HA witness client: %w", err)
	}
	controller, err := haauthority.New(client)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("configure HA dispatch authority: %w", err)
	}
	supervisor, err := haauthority.NewSupervisor(controller, sessions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("configure HA authority supervisor: %w", err)
	}
	return dispatchauthority.New(registry, controller), controller, supervisor, nil
}
