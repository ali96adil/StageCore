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

// HAAuthority is the product-facing optional HA surface. Fresh authority can be
// obtained only through Activate, which applies the Session guard and starts
// same-epoch renewal. Current is read-only compatibility/inspection; there is
// deliberately no generic Renew or auto-acquire method on this surface.
type HAAuthority interface {
	Current(context.Context) (dispatchauthority.Snapshot, error)
	Status(context.Context) (haauthority.SupervisorStatus, error)
	Activate(context.Context) error
	Release(context.Context) error
	Demote()
	Close() error
}

type supervisedHAAuthority struct {
	controller *haauthority.Controller
	supervisor *haauthority.Supervisor
}

func (a *supervisedHAAuthority) Current(ctx context.Context) (dispatchauthority.Snapshot, error) {
	return a.controller.Current(ctx)
}

func (a *supervisedHAAuthority) Status(ctx context.Context) (haauthority.SupervisorStatus, error) {
	return a.supervisor.Status(ctx)
}

func (a *supervisedHAAuthority) Activate(ctx context.Context) error {
	return a.supervisor.Activate(ctx)
}

func (a *supervisedHAAuthority) Release(ctx context.Context) error {
	return a.supervisor.Release(ctx)
}

func (a *supervisedHAAuthority) Demote() { a.supervisor.Demote() }

func (a *supervisedHAAuthority) Close() error { return a.supervisor.Close() }

func physicalDispatchForHA(
	ctx context.Context,
	cfg config.Config,
	registry *capability.Registry,
	hubSecurity *hubsecurity.Service,
	sessions haauthority.SessionReader,
) (capability.Executor, HAAuthority, error) {
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
	if sessions == nil {
		return nil, nil, fmt.Errorf("HA Session authority is required for witness mode")
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
	supervisor, err := haauthority.NewSupervisor(controller, sessions)
	if err != nil {
		return nil, nil, fmt.Errorf("configure HA authority supervisor: %w", err)
	}
	productAuthority := &supervisedHAAuthority{controller: controller, supervisor: supervisor}
	return dispatchauthority.New(registry, controller), productAuthority, nil
}
