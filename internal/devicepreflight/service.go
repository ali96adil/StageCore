package devicepreflight

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/preflight"
)

const defaultNetworkStaleAfter = 15 * time.Second

type Base interface {
	Evaluate(context.Context, string, string) (preflight.Report, error)
}

type Service struct {
	base       Base
	repository *deviceexperience.Repository
	staleAfter time.Duration
}

func New(base Base, repository *deviceexperience.Repository) *Service {
	return &Service{base: base, repository: repository, staleAfter: defaultNetworkStaleAfter}
}

func (s *Service) Evaluate(ctx context.Context, projectID, runtimeSnapshotID string) (preflight.Report, error) {
	if s == nil || s.base == nil || s.repository == nil {
		return preflight.Report{}, fmt.Errorf("stage device preflight is unavailable")
	}
	report, err := s.base.Evaluate(ctx, projectID, runtimeSnapshotID)
	if err != nil {
		return preflight.Report{}, err
	}
	if strings.TrimSpace(report.RuntimeSnapshotID) == "" {
		return report, nil
	}

	devices, err := s.repository.ListDevices(ctx, report.ProjectID)
	if err != nil {
		return preflight.Report{}, fmt.Errorf("list stage devices for preflight: %w", err)
	}
	deviceByID := make(map[string]deviceexperience.Device, len(devices))
	for _, device := range devices {
		deviceByID[device.ID] = device
		if !device.Enabled {
			continue
		}
		status, summary, detail := deviceStatus(device)
		add(&report, status, "device."+device.ID, "stage_device", summary, detail, device.ID)
	}

	sources, err := s.repository.ListLiveSources(ctx, report.ProjectID)
	if err != nil {
		return preflight.Report{}, fmt.Errorf("list live video sources for preflight: %w", err)
	}
	sourceByID := make(map[string]deviceexperience.LiveSource, len(sources))
	endpointRefs := make(map[string]bool, len(sources))
	for _, source := range sources {
		sourceByID[source.ID] = source
		if strings.TrimSpace(source.EndpointRef) != "" {
			endpointRefs[strings.TrimSpace(source.EndpointRef)] = source.Required
		}
		evaluateSource(&report, source, deviceByID)
	}

	cockpit, err := s.repository.Cockpit(ctx, s.staleAfter)
	if err != nil {
		return preflight.Report{}, fmt.Errorf("load Stage Network Cockpit for preflight: %w", err)
	}
	for _, target := range cockpit {
		if !projectNetworkTarget(target, deviceByID, sourceByID, endpointRefs) {
			continue
		}
		status := preflight.Pass
		if target.Readiness != deviceexperience.ReadinessReady {
			status = preflight.Warn
		}
		detail := target.ReasonCode
		if target.Observation.TransportState != "" {
			if detail != "" {
				detail += " · "
			}
			detail += target.Observation.TransportState
		}
		add(&report, status, "network."+target.TargetKind+"."+target.TargetID, "network", "Stage network observation: "+target.TargetID, detail, target.TargetID)
	}
	return report, nil
}

func (s *Service) ShowGate(ctx context.Context, projectID, runtimeSnapshotID string) (bool, string, error) {
	report, err := s.Evaluate(ctx, projectID, runtimeSnapshotID)
	if err != nil {
		return false, "", err
	}
	if report.Status != preflight.Block {
		return true, "", nil
	}
	for _, check := range report.Checks {
		if check.Status == preflight.Block {
			return false, check.Summary, nil
		}
	}
	return false, "SHOW Preflight contains a blocking Stage Device condition", nil
}

func deviceStatus(device deviceexperience.Device) (preflight.Status, string, string) {
	name := strings.TrimSpace(device.DisplayName)
	if name == "" {
		name = device.ID
	}
	if device.Runtime == nil {
		return preflight.Warn, "Stage Device has no runtime observation: " + name, "No authenticated runtime state has been observed."
	}
	if device.Runtime.Connection != deviceexperience.ConnectionOnline {
		return preflight.Warn, "Stage Device is not online: " + name, string(device.Runtime.Connection)
	}
	if device.Runtime.Readiness != deviceexperience.ReadinessReady {
		return preflight.Warn, "Stage Device is online but not READY: " + name, string(device.Runtime.Readiness)
	}
	return preflight.Pass, "Stage Device is READY: " + name, "Authenticated runtime channel is online."
}

func evaluateSource(report *preflight.Report, source deviceexperience.LiveSource, deviceByID map[string]deviceexperience.Device) {
	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = source.ID
	}
	key := "live_video." + source.ID
	if source.Required && !source.DesiredEnabled {
		add(report, preflight.Block, key, "live_video", "Required live-video source is disabled: "+name, "Enable the required source or remove its Required-for-show designation before SHOW.", source.ID)
		return
	}
	if !source.DesiredEnabled {
		add(report, preflight.Pass, key, "live_video", "Optional live-video source is disabled: "+name, "Disabled optional source does not block SHOW.", source.ID)
		return
	}
	if source.ExecutionDeviceID != "" {
		device, ok := deviceByID[source.ExecutionDeviceID]
		if !ok || !device.Enabled || device.Runtime == nil || device.Runtime.Connection != deviceexperience.ConnectionOnline {
			status := preflight.Warn
			if source.Required {
				status = preflight.Block
			}
			add(report, status, key+".execution_device", "live_video", "Live-video Render Node is unavailable: "+name, source.ExecutionDeviceID, source.ID)
		} else if missing := missingCapabilities(source.Capabilities, device.Capabilities); len(missing) > 0 {
			status := preflight.Warn
			if source.Required {
				status = preflight.Block
			}
			add(report, status, key+".capabilities", "live_video", "Live-video Render Node lacks required capabilities: "+name, strings.Join(missing, ", "), source.ID)
		}
	}
	if source.Readiness == deviceexperience.ReadinessReady {
		add(report, preflight.Pass, key, "live_video", "Live-video source is READY: "+name, string(source.Class), source.ID)
		return
	}
	status := preflight.Warn
	if source.Required {
		status = preflight.Block
	}
	add(report, status, key, "live_video", "Live-video source is not READY: "+name, string(source.Readiness), source.ID)
}

func missingCapabilities(required, advertised []string) []string {
	available := make(map[string]struct{}, len(advertised))
	for _, capability := range advertised {
		capability = strings.TrimSpace(capability)
		if capability != "" {
			available[capability] = struct{}{}
		}
	}
	missing := make([]string, 0)
	seen := make(map[string]struct{})
	for _, capability := range required {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			continue
		}
		if _, duplicate := seen[capability]; duplicate {
			continue
		}
		seen[capability] = struct{}{}
		if _, ok := available[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	sort.Strings(missing)
	return missing
}

func projectNetworkTarget(target deviceexperience.CockpitTarget, devices map[string]deviceexperience.Device, sources map[string]deviceexperience.LiveSource, endpoints map[string]bool) bool {
	switch target.TargetKind {
	case "STAGE_DEVICE":
		_, ok := devices[target.TargetID]
		return ok
	case "LIVE_SOURCE":
		_, ok := sources[target.TargetID]
		return ok
	case "ENDPOINT":
		_, ok := endpoints[target.TargetID]
		return ok
	default:
		return false
	}
}

func add(report *preflight.Report, status preflight.Status, key, category, summary, detail, entityID string) {
	report.Checks = append(report.Checks, preflight.Check{Key: key, Category: category, Status: status, Summary: summary, Detail: detail, EntityID: entityID})
	if rank(status) > rank(report.Status) {
		report.Status = status
	}
}

func rank(status preflight.Status) int {
	switch status {
	case preflight.Pass:
		return 0
	case preflight.Warn:
		return 1
	default:
		return 2
	}
}
