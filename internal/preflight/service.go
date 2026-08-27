package preflight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/store"
)

type Status string

const (
	Pass  Status = "PASS"
	Warn  Status = "WARN"
	Block Status = "BLOCK"
)

type Check struct {
	Key      string `json:"key"`
	Category string `json:"category"`
	Status   Status `json:"status"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail,omitempty"`
	EntityID string `json:"entity_id,omitempty"`
}

type RoleStatus struct {
	MachineRoleID           string                     `json:"machine_role_id"`
	RoleKey                 string                     `json:"role_key"`
	Required                bool                       `json:"required"`
	AssignmentID            string                     `json:"assignment_id,omitempty"`
	CompanionID             string                     `json:"companion_id,omitempty"`
	CompanionName           string                     `json:"companion_name,omitempty"`
	Connected               bool                       `json:"connected"`
	TrustState              domain.CompanionTrustState `json:"trust_state,omitempty"`
	Readiness               domain.CompanionReadiness  `json:"readiness,omitempty"`
	RoleState               domain.RoleAssignmentState `json:"role_state"`
	AppliedRuntimeSnapshotID string                     `json:"applied_runtime_snapshot_id,omitempty"`
	Status                   Status                     `json:"status"`
	Summary                  string                     `json:"summary"`
}

type MediaStatus struct {
	MachineRoleID   string `json:"machine_role_id"`
	RoleKey         string `json:"role_key"`
	MediaAssetID    string `json:"media_asset_id"`
	ContentVersionID string `json:"content_version_id"`
	ContentHash     string `json:"content_hash"`
	SizeBytes       int64  `json:"size_bytes"`
	Required        bool   `json:"required"`
	Status          Status `json:"status"`
	Summary         string `json:"summary"`
}

type StorageStatus struct {
	State        storagehealth.State `json:"state"`
	Status       Status              `json:"status"`
	FreeBytes    uint64              `json:"free_bytes"`
	ReserveBytes uint64              `json:"reserve_bytes"`
	FreePercent  float64             `json:"free_percent"`
	Writable     bool                `json:"writable"`
	Reason       string              `json:"reason,omitempty"`
}

type Report struct {
	Status          Status        `json:"status"`
	ProjectID       string        `json:"project_id"`
	RuntimeSnapshotID string      `json:"runtime_snapshot_id,omitempty"`
	SnapshotVersion int64         `json:"snapshot_version,omitempty"`
	EvaluatedAt     time.Time     `json:"evaluated_at"`
	Checks          []Check       `json:"checks"`
	Roles           []RoleStatus  `json:"roles"`
	Media           []MediaStatus `json:"media"`
	Storage         StorageStatus `json:"storage"`
}

func (r Report) AllowsShow() bool { return r.Status != Block }

type ConnectionCheck func(string) bool

type Service struct {
	store            *store.Store
	capabilities     *capability.Registry
	storage          *storagehealth.Monitor
	connected        ConnectionCheck
	now              func() time.Time
	heartbeatTimeout time.Duration
}

type Option func(*Service)

func WithConnectionCheck(check ConnectionCheck) Option {
	return func(s *Service) { s.connected = check }
}

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithHeartbeatTimeout(timeout time.Duration) Option {
	return func(s *Service) {
		if timeout > 0 {
			s.heartbeatTimeout = timeout
		}
	}
}

func New(s *store.Store, capabilities *capability.Registry, storage *storagehealth.Monitor, options ...Option) *Service {
	service := &Service{
		store: s, capabilities: capabilities, storage: storage,
		now: time.Now, heartbeatTimeout: 5 * time.Second,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Evaluate(ctx context.Context, projectID, runtimeSnapshotID string) (Report, error) {
	report := Report{
		Status: Pass, ProjectID: strings.TrimSpace(projectID), EvaluatedAt: s.now().UTC(),
		Checks: []Check{}, Roles: []RoleStatus{}, Media: []MediaStatus{},
	}
	if s == nil || s.store == nil || s.capabilities == nil || s.storage == nil {
		return Report{}, fmt.Errorf("preflight service is unavailable")
	}
	project, err := s.store.GetProject(ctx, report.ProjectID)
	if err != nil {
		return Report{}, err
	}

	runtimeSnapshot, err := s.resolveSnapshot(ctx, project.ID, runtimeSnapshotID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			report.add(Block, "snapshot.exists", "snapshot", "Published Runtime Snapshot required", "No published Runtime Snapshot is available for this Project.", project.ID)
			s.evaluateStorage(&report)
			return report, nil
		}
		return Report{}, err
	}
	report.RuntimeSnapshotID = runtimeSnapshot.ID
	report.SnapshotVersion = runtimeSnapshot.SnapshotVersion

	manifest, snapshotUsable := s.evaluateSnapshot(ctx, &report, project, runtimeSnapshot)
	if snapshotUsable {
		s.evaluateCapabilitiesAndRoles(ctx, &report, manifest, runtimeSnapshot)
		s.evaluateMedia(&report, manifest)
	}
	s.evaluateStorage(&report)
	sort.SliceStable(report.Checks, func(i, j int) bool {
		if report.Checks[i].Category == report.Checks[j].Category {
			return report.Checks[i].Key < report.Checks[j].Key
		}
		return report.Checks[i].Category < report.Checks[j].Category
	})
	return report, nil
}

func (s *Service) ShowGate(ctx context.Context, projectID, runtimeSnapshotID string) (bool, string, error) {
	report, err := s.Evaluate(ctx, projectID, runtimeSnapshotID)
	if err != nil {
		return false, "", err
	}
	if report.AllowsShow() {
		return true, "", nil
	}
	for _, check := range report.Checks {
		if check.Status == Block {
			return false, check.Summary, nil
		}
	}
	return false, "SHOW Preflight contains a blocking condition", nil
}

func (s *Service) resolveSnapshot(ctx context.Context, projectID, runtimeSnapshotID string) (domain.RuntimeSnapshot, error) {
	if strings.TrimSpace(runtimeSnapshotID) != "" {
		return s.store.GetRuntimeSnapshot(ctx, strings.TrimSpace(runtimeSnapshotID))
	}
	latest, err := s.store.LatestPublishedRuntimeSnapshotForProject(ctx, projectID)
	if err != nil {
		return domain.RuntimeSnapshot{}, err
	}
	if latest == nil {
		return domain.RuntimeSnapshot{}, domain.ErrNotFound
	}
	return *latest, nil
}

func (s *Service) evaluateSnapshot(ctx context.Context, report *Report, project domain.Project, runtimeSnapshot domain.RuntimeSnapshot) (snapshot.Manifest, bool) {
	usable := true
	if runtimeSnapshot.Status != domain.SnapshotPublished {
		report.add(Block, "snapshot.published", "snapshot", "Runtime Snapshot is not published", string(runtimeSnapshot.Status), runtimeSnapshot.ID)
		usable = false
	} else {
		report.add(Pass, "snapshot.published", "snapshot", "Runtime Snapshot is published", fmt.Sprintf("Snapshot v%d", runtimeSnapshot.SnapshotVersion), runtimeSnapshot.ID)
	}
	if runtimeSnapshot.ProjectID != project.ID {
		report.add(Block, "snapshot.project", "snapshot", "Runtime Snapshot belongs to another Project", runtimeSnapshot.ProjectID, runtimeSnapshot.ID)
		usable = false
	} else {
		report.add(Pass, "snapshot.project", "snapshot", "Runtime Snapshot matches Project", project.Name, runtimeSnapshot.ID)
	}
	digest := sha256.Sum256(runtimeSnapshot.Manifest)
	actualHash := hex.EncodeToString(digest[:])
	if runtimeSnapshot.ContentHash == "" || !strings.EqualFold(runtimeSnapshot.ContentHash, actualHash) {
		report.add(Block, "snapshot.integrity", "snapshot", "Runtime Snapshot manifest integrity mismatch", "Stored content hash does not match the immutable manifest bytes.", runtimeSnapshot.ID)
		usable = false
	} else {
		report.add(Pass, "snapshot.integrity", "snapshot", "Runtime Snapshot integrity verified", actualHash, runtimeSnapshot.ID)
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		report.add(Block, "snapshot.schema", "snapshot", "Runtime Snapshot manifest cannot be decoded", err.Error(), runtimeSnapshot.ID)
		return snapshot.Manifest{}, false
	}
	if manifest.ProjectID != project.ID || manifest.RevisionID != runtimeSnapshot.RevisionID {
		report.add(Block, "snapshot.identity", "snapshot", "Runtime Snapshot manifest identity mismatch", "Manifest Project/Revision identity does not match the stored Runtime Snapshot.", runtimeSnapshot.ID)
		usable = false
	} else {
		report.add(Pass, "snapshot.identity", "snapshot", "Runtime Snapshot manifest identity matches", fmt.Sprintf("revision %s", manifest.RevisionID), runtimeSnapshot.ID)
	}
	revision, err := s.store.GetRevision(ctx, runtimeSnapshot.RevisionID)
	if err != nil || revision.ProjectID != project.ID || revision.Status != domain.RevisionValidated {
		detail := "Published Snapshot revision is not a VALIDATED revision of this Project."
		if err != nil {
			detail = err.Error()
		}
		report.add(Block, "snapshot.revision", "snapshot", "Published revision consistency failed", detail, runtimeSnapshot.RevisionID)
		usable = false
	} else {
		report.add(Pass, "snapshot.revision", "snapshot", "Published revision is validated", fmt.Sprintf("revision %d", revision.RevisionNumber), revision.ID)
	}
	return manifest, usable
}

type roleDependency struct {
	roleID       string
	roleKey      string
	required     bool
	capabilities map[string]struct{}
}

func (s *Service) evaluateCapabilitiesAndRoles(ctx context.Context, report *Report, manifest snapshot.Manifest, runtimeSnapshot domain.RuntimeSnapshot) {
	targetByRef := make(map[string]snapshot.Target, len(manifest.Targets))
	for _, target := range manifest.Targets {
		targetByRef[target.TargetRef] = target
	}
	dependencies := map[string]*roleDependency{}
	checkCapability := func(targetRef, capabilityKey, entityID string) {
		target, ok := targetByRef[targetRef]
		if !ok {
			report.add(Block, "target."+entityID, "adapter", "Runtime target is missing from Snapshot", targetRef, entityID)
			return
		}
		if !s.capabilities.Supports(capabilityKey, target.LogicalType) {
			report.add(Block, "capability."+entityID, "adapter", "Required runtime adapter is unavailable", capabilityKey+" @ "+targetRef, entityID)
			return
		}
		report.add(Pass, "capability."+entityID, "adapter", "Required runtime adapter is available", capabilityKey+" @ "+targetRef, entityID)
		if !strings.EqualFold(strings.TrimSpace(target.LogicalType), companion.MachineRoleLogicalType) {
			if capabilityKey == "osc.send" {
				report.add(Warn, "target.unverified."+entityID, "adapter", "OSC target availability is not independently verified", "Configuration is executable, but UDP transport alone cannot prove the physical endpoint is online.", entityID)
			}
			return
		}
		var cfg struct {
			MachineRoleID string `json:"machine_role_id"`
		}
		if err := json.Unmarshal(target.Configuration, &cfg); err != nil || strings.TrimSpace(cfg.MachineRoleID) == "" {
			report.add(Block, "role.target."+entityID, "companion", "Machine Role target configuration is invalid", targetRef, entityID)
			return
		}
		dep := dependencies[cfg.MachineRoleID]
		if dep == nil {
			dep = &roleDependency{roleID: cfg.MachineRoleID, roleKey: target.TargetRef, required: true, capabilities: map[string]struct{}{}}
			dependencies[cfg.MachineRoleID] = dep
		}
		dep.required = true
		dep.capabilities[capabilityKey] = struct{}{}
	}
	for _, cue := range manifest.Cues {
		if !cue.Enabled {
			continue
		}
		for _, action := range cue.Actions {
			if action.Enabled {
				checkCapability(action.TargetRef, action.CapabilityKey, action.ID)
			}
		}
	}
	outputByID := make(map[string]snapshot.Output, len(manifest.Outputs))
	for _, output := range manifest.Outputs {
		outputByID[output.ID] = output
	}
	for _, route := range manifest.Routes {
		if !route.Enabled {
			continue
		}
		for _, action := range route.Actions {
			if action.OutputID == nil {
				continue
			}
			if output, ok := outputByID[*action.OutputID]; ok {
				checkCapability(output.TargetRef, output.CapabilityKey, output.ID)
			}
		}
	}
	for _, media := range manifest.RequiredMedia {
		if strings.TrimSpace(media.MachineRoleID) == "" {
			continue
		}
		dep := dependencies[media.MachineRoleID]
		if dep == nil {
			dep = &roleDependency{roleID: media.MachineRoleID, roleKey: media.RoleKey, required: media.Required, capabilities: map[string]struct{}{}}
			dependencies[media.MachineRoleID] = dep
		}
		if media.Required {
			dep.required = true
		}
	}

	ids := make([]string, 0, len(dependencies))
	for id := range dependencies {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		s.evaluateRole(ctx, report, dependencies[id], runtimeSnapshot)
	}
}

func (s *Service) evaluateRole(ctx context.Context, report *Report, dependency *roleDependency, runtimeSnapshot domain.RuntimeSnapshot) {
	roleView := RoleStatus{MachineRoleID: dependency.roleID, RoleKey: dependency.roleKey, Required: dependency.required, RoleState: domain.RoleUnassigned, Status: Pass}
	role, err := s.store.GetMachineRole(ctx, dependency.roleID)
	if err != nil {
		roleView.Status = severityForRequired(dependency.required)
		roleView.Summary = "Machine Role is not available"
		report.Roles = append(report.Roles, roleView)
		report.add(roleView.Status, "role."+dependency.roleID, "companion", roleView.Summary, dependency.roleKey, dependency.roleID)
		return
	}
	roleView.RoleKey = role.RoleKey
	if role.ProjectID != runtimeSnapshot.ProjectID || (dependency.roleKey != "" && role.RoleKey != dependency.roleKey) {
		roleView.Status = Block
		roleView.Summary = "Machine Role does not match Snapshot target"
		report.Roles = append(report.Roles, roleView)
		report.add(Block, "role.identity."+role.ID, "companion", roleView.Summary, role.RoleKey, role.ID)
		return
	}
	assignment, err := s.store.GetActiveRoleAssignment(ctx, role.ID)
	if err != nil {
		roleView.Status = severityForRequired(dependency.required)
		roleView.Summary = "Machine Role has no active Companion assignment"
		report.Roles = append(report.Roles, roleView)
		report.add(roleView.Status, "role.assignment."+role.ID, "companion", roleView.Summary, role.RoleKey, role.ID)
		return
	}
	roleView.AssignmentID = assignment.ID
	roleView.CompanionID = assignment.CompanionID
	companionState, err := s.store.GetCompanion(ctx, assignment.CompanionID)
	if err != nil {
		roleView.Status = severityForRequired(dependency.required)
		roleView.Summary = "Assigned Companion is unavailable"
		report.Roles = append(report.Roles, roleView)
		report.add(roleView.Status, "role.companion."+role.ID, "companion", roleView.Summary, assignment.CompanionID, role.ID)
		return
	}
	roleView.CompanionName = companionState.DisplayName
	roleView.TrustState = companionState.TrustState
	roleView.Readiness = companionState.Readiness
	if companionState.AppliedRuntimeSnapshotID != nil {
		roleView.AppliedRuntimeSnapshotID = *companionState.AppliedRuntimeSnapshotID
	}
	roleView.Connected = true
	if s.connected != nil {
		roleView.Connected = s.connected(companionState.ID)
	}

	effectiveRole := role
	requiredCapabilities := make(map[string]struct{}, len(role.RequiredCapabilities)+len(dependency.capabilities))
	for _, key := range role.RequiredCapabilities {
		requiredCapabilities[key] = struct{}{}
	}
	for key := range dependency.capabilities {
		requiredCapabilities[key] = struct{}{}
	}
	effectiveRole.RequiredCapabilities = effectiveRole.RequiredCapabilities[:0]
	for key := range requiredCapabilities {
		effectiveRole.RequiredCapabilities = append(effectiveRole.RequiredCapabilities, key)
	}
	sort.Strings(effectiveRole.RequiredCapabilities)
	expectedSnapshotID := runtimeSnapshot.ID
	effectiveRole.RequiredRuntimeSnapshotID = &expectedSnapshotID
	evaluation := companion.EvaluateRole(effectiveRole, companionState, report.EvaluatedAt, s.heartbeatTimeout)
	roleView.RoleState = evaluation.RoleState
	if !roleView.Connected {
		roleView.RoleState = domain.RoleOffline
		roleView.Readiness = domain.CompanionReadinessOffline
	}
	if roleView.Connected && roleView.RoleState == domain.RoleReady {
		roleView.Status = Pass
		roleView.Summary = "Machine Role is READY on the required Runtime Snapshot"
	} else {
		roleView.Status = severityForRequired(dependency.required)
		roleView.Summary = fmt.Sprintf("Machine Role is %s", roleView.RoleState)
		if !roleView.Connected {
			roleView.Summary = "Assigned Companion runtime connection is offline"
		}
	}
	report.Roles = append(report.Roles, roleView)
	detail := roleView.CompanionName
	if detail == "" {
		detail = roleView.CompanionID
	}
	report.add(roleView.Status, "role.readiness."+role.ID, "companion", roleView.Summary, detail, role.ID)
}

func (s *Service) evaluateMedia(report *Report, manifest snapshot.Manifest) {
	roleByID := make(map[string]RoleStatus, len(report.Roles))
	for _, role := range report.Roles {
		roleByID[role.MachineRoleID] = role
	}
	for _, requirement := range manifest.RequiredMedia {
		status := Warn
		summary := "Media readiness cannot be verified without a READY assigned Companion"
		if role, ok := roleByID[requirement.MachineRoleID]; ok && role.Status == Pass && role.Readiness == domain.CompanionReadinessReady && role.AppliedRuntimeSnapshotID == report.RuntimeSnapshotID {
			status = Pass
			summary = "Assigned Companion reports READY after exact media checksum verification"
		} else if requirement.Required {
			status = Block
		}
		report.Media = append(report.Media, MediaStatus{
			MachineRoleID: requirement.MachineRoleID, RoleKey: requirement.RoleKey,
			MediaAssetID: requirement.MediaAssetID, ContentVersionID: requirement.ContentVersionID,
			ContentHash: requirement.ContentHash, SizeBytes: requirement.SizeBytes,
			Required: requirement.Required, Status: status, Summary: summary,
		})
		report.add(status, "media."+requirement.ContentVersionID, "media", summary, requirement.ContentHash, requirement.MediaAssetID)
	}
}

func (s *Service) evaluateStorage(report *Report) {
	aggregate := s.storage.Status()
	worst := aggregate.DataRoot
	if storageRank(aggregate.VaultRoot.State) > storageRank(worst.State) {
		worst = aggregate.VaultRoot
	}
	status := Pass
	switch aggregate.State {
	case storagehealth.Healthy:
		status = Pass
	case storagehealth.Warning:
		status = Warn
	default:
		status = Block
	}
	report.Storage = StorageStatus{
		State: aggregate.State, Status: status, FreeBytes: worst.FreeBytes,
		ReserveBytes: worst.ReserveBytes, FreePercent: worst.FreePercent,
		Writable: worst.Writable, Reason: worst.Reason,
	}
	summary := "Authoritative storage is healthy"
	if status == Warn {
		summary = "Authoritative storage is below the warning threshold"
	} else if status == Block {
		summary = "Authoritative storage cannot guarantee the runtime reserve/logging boundary"
	}
	report.add(status, "storage.authoritative", "storage", summary, worst.Reason, "")
}

func (r *Report) add(status Status, key, category, summary, detail, entityID string) {
	r.Checks = append(r.Checks, Check{Key: key, Category: category, Status: status, Summary: summary, Detail: detail, EntityID: entityID})
	if statusRank(status) > statusRank(r.Status) {
		r.Status = status
	}
}

func severityForRequired(required bool) Status {
	if required {
		return Block
	}
	return Warn
}

func statusRank(status Status) int {
	switch status {
	case Pass:
		return 0
	case Warn:
		return 1
	case Block:
		return 2
	default:
		return 2
	}
}

func storageRank(state storagehealth.State) int {
	switch state {
	case storagehealth.Healthy:
		return 0
	case storagehealth.Warning:
		return 1
	case storagehealth.Degraded:
		return 2
	case storagehealth.Critical:
		return 3
	case storagehealth.Unavailable:
		return 4
	default:
		return 4
	}
}
