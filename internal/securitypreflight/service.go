package securitypreflight

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginpermissions"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/secretstore"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type Service struct {
	base    *preflight.Service
	store   *store.Store
	hub     *hubsecurity.Service
	secrets *secretstore.Service
	plugins *pluginpermissions.Service
}

func New(base *preflight.Service, stageStore *store.Store, hub *hubsecurity.Service, secrets *secretstore.Service, plugins *pluginpermissions.Service) *Service {
	return &Service{base: base, store: stageStore, hub: hub, secrets: secrets, plugins: plugins}
}

func (s *Service) Evaluate(ctx context.Context, projectID, runtimeSnapshotID string) (preflight.Report, error) {
	if s == nil || s.base == nil || s.store == nil || s.hub == nil || s.secrets == nil || s.plugins == nil {
		return preflight.Report{}, fmt.Errorf("security preflight is unavailable")
	}
	report, err := s.base.Evaluate(ctx, projectID, runtimeSnapshotID)
	if err != nil {
		return preflight.Report{}, err
	}
	identity, err := s.hub.Identity(ctx)
	if err != nil || identity.BootstrapState != hubsecurity.BootstrapClaimed {
		add(&report, preflight.Block, "security.hub.claimed", "security", "Hub security identity is not claimed", "OWNER bootstrap/security identity must be healthy before SHOW.", "")
	} else {
		add(&report, preflight.Pass, "security.hub.claimed", "security", "Hub security identity is claimed", identity.Fingerprint, identity.HubID)
	}
	if strings.TrimSpace(report.RuntimeSnapshotID) == "" {
		return report, nil
	}
	runtimeSnapshot, err := s.store.GetRuntimeSnapshot(ctx, report.RuntimeSnapshotID)
	if err != nil {
		return preflight.Report{}, err
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return report, nil // base Preflight already owns the manifest/schema blocker.
	}
	s.evaluateSnapshotSecurity(ctx, &report, manifest)
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
	return false, "SHOW Preflight contains a blocking security condition", nil
}

func (s *Service) evaluateSnapshotSecurity(ctx context.Context, report *preflight.Report, manifest snapshot.Manifest) {
	targets := make(map[string]snapshot.Target, len(manifest.Targets))
	for _, target := range manifest.Targets {
		targets[target.TargetRef] = target
	}
	requiredTargets := map[string]map[string]struct{}{}
	mark := func(targetRef, capability string) {
		if requiredTargets[targetRef] == nil {
			requiredTargets[targetRef] = map[string]struct{}{}
		}
		requiredTargets[targetRef][capability] = struct{}{}
	}
	for _, cue := range manifest.Cues {
		if !cue.Enabled {
			continue
		}
		for _, action := range cue.Actions {
			if action.Enabled {
				mark(action.TargetRef, action.CapabilityKey)
			}
		}
	}
	outputs := make(map[string]snapshot.Output, len(manifest.Outputs))
	for _, output := range manifest.Outputs {
		outputs[output.ID] = output
	}
	for _, route := range manifest.Routes {
		if !route.Enabled {
			continue
		}
		for _, action := range route.Actions {
			if action.OutputID == nil {
				continue
			}
			if output, ok := outputs[*action.OutputID]; ok {
				mark(output.TargetRef, output.CapabilityKey)
			}
		}
	}

	secretRefs := map[string]struct{}{}
	needsOSCPlugin := false
	for targetRef, capabilities := range requiredTargets {
		target, ok := targets[targetRef]
		if !ok {
			continue
		}
		collectSecretReferences(target.Configuration, secretRefs)
		if _, usesOSC := capabilities[oscplugin.CapabilityOSCSend]; usesOSC && !strings.EqualFold(strings.TrimSpace(target.LogicalType), companion.MachineRoleLogicalType) {
			needsOSCPlugin = true
		}
		if hasInlineCredential(target.Configuration) {
			add(report, preflight.Block, "security.secret.inline."+target.AliasID, "security", "Runtime target contains inline credential-like configuration", "Use a stable secret_ref instead of embedding credentials in Project/Snapshot data.", target.AliasID)
		}
	}

	for reference := range secretRefs {
		value, err := s.secrets.Resolve(ctx, reference)
		if err != nil || value == "" {
			add(report, preflight.Block, "security.secret."+reference, "security", "Required Secret Store reference is unavailable", reference, reference)
			continue
		}
		add(report, preflight.Pass, "security.secret."+reference, "security", "Required Secret Store reference is readable", reference, reference)
	}

	if needsOSCPlugin {
		granted, err := s.plugins.Granted(ctx, oscplugin.PluginID)
		if err != nil {
			add(report, preflight.Block, "security.plugin.osc", "security", "OSC Plugin permission state is unavailable", err.Error(), oscplugin.PluginID)
			return
		}
		if !contains(granted, oscplugin.PermissionUDPSend) {
			add(report, preflight.Block, "security.plugin.osc.udp-send", "security", "OSC Plugin lacks required UDP send permission", oscplugin.PermissionUDPSend, oscplugin.PluginID)
			return
		}
		add(report, preflight.Pass, "security.plugin.osc.udp-send", "security", "OSC Plugin has required UDP send permission", oscplugin.PermissionUDPSend, oscplugin.PluginID)
	}
}

func collectSecretReferences(raw json.RawMessage, out map[string]struct{}) {
	if len(raw) == 0 {
		return
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return
	}
	walk(value, func(key string, value any) {
		text, ok := value.(string)
		if !ok {
			return
		}
		if strings.EqualFold(key, "secret_ref") && strings.HasPrefix(strings.TrimSpace(text), "secret:") {
			out[strings.TrimSpace(text)] = struct{}{}
		}
	})
}

func hasInlineCredential(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	found := false
	walk(value, func(key string, value any) {
		if found || strings.EqualFold(key, "secret_ref") || strings.EqualFold(key, "secret_header") || strings.EqualFold(key, "secret_prefix") {
			return
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "authorization" || key == "password" || key == "token" || key == "api_key" || key == "apikey" || key == "secret" || strings.HasSuffix(key, "_password") || strings.HasSuffix(key, "_token") || strings.HasSuffix(key, "_secret") {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				found = true
			}
		}
	})
	return found
}

func walk(value any, visit func(string, any)) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			visit(key, child)
			walk(child, visit)
		}
	case []any:
		for _, child := range item {
			walk(child, visit)
		}
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

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
