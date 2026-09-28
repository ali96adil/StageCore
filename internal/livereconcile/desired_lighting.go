// Package livereconcile derives conservative, read-only desired lighting state
// from the active Hub session and immutable published Runtime Snapshot.
// It never activates a v2 device, dispatches a GO or authorizes DMX output.
package livereconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

var ErrDesiredUncertain = errors.New("current LIVE lighting desired state is not fully established")

// SessionStore is the existing Hub read model. A fake implementation can
// exercise GO races without running hardware or mutating show history.
type SessionStore interface {
	ActiveSessionForProject(context.Context, string) (*domain.Session, error)
	GetSession(context.Context, string) (domain.Session, error)
	GetRuntimeSnapshot(context.Context, string) (domain.RuntimeSnapshot, error)
	ListCueExecutions(context.Context, string) ([]domain.CueExecution, error)
	HasRunningCueExecution(context.Context, string) (bool, error)
}

var _ SessionStore = (*store.Store)(nil)

type Service struct{ sessions SessionStore }

func NewService(sessions SessionStore) *Service { return &Service{sessions: sessions} }

type DesiredLightingMode string

const (
	DesiredLightingChannelLevels DesiredLightingMode = "CHANNEL_LEVELS"
	DesiredLightingBlackout      DesiredLightingMode = "BLACKOUT"
)

// DesiredLightingTarget preserves the exact logical target authored by the
// current Cue alongside the pinned configuration's resulting DMX byte.
// Keeping both prevents unsafe reverse conversion from an observed DMX byte
// back to a logical percentage (which can be ambiguous after clamp/inversion/
// quantization).
type DesiredLightingTarget struct {
	ChannelKey   string
	LogicalLevel float64
	DMXValue     uint8
}

// CueLightingProjection is the pure current-Cue lighting projection. Channels
// remains the software-DMX comparison surface; Targets preserves the exact
// logical values needed to materialize a later state correction without
// replaying the historical Cue action.
type CueLightingProjection struct {
	Mode     DesiredLightingMode
	Channels map[int]uint8
	Targets  map[int]DesiredLightingTarget
}

// DesiredLighting is an immutable-by-convention read model. Channels holds
// the target converted through the pinned snapshot's channel binding to
// software DMX slots. Targets retains exact authored logical channel values.
// Neither is independently observed LED/decoder state.
type DesiredLighting struct {
	ProjectID      string
	SessionID      string
	SnapshotID     string
	SnapshotHash   string
	CueID          string
	CueExecutionID string
	Mode           DesiredLightingMode
	Channels       map[int]uint8
	Targets        map[int]DesiredLightingTarget
}

// ReadCurrentLighting resolves only the current *completed* Cue and requires
// every enabled, configured channel on the device to be deterministically
// addressed there. We deliberately refuse partial Cue deltas: an unchanged
// channel might depend on a prior GO or local override that cannot be
// reconstructed just by reading the current Cue. The returned object has
// no assignment epoch/socket/activation authority. Before future dispatch,
// callers MUST independently revalidate every LIVE scope and safety gate.
func (s *Service) ReadCurrentLighting(ctx context.Context, projectID, deviceID string) (DesiredLighting, error) {
	fail := func(reason string) (DesiredLighting, error) {
		return DesiredLighting{}, fmt.Errorf("%w: %s", ErrDesiredUncertain, reason)
	}
	projectID, deviceID = strings.TrimSpace(projectID), strings.TrimSpace(deviceID)
	if s == nil || s.sessions == nil || ctx == nil || projectID == "" || deviceID == "" {
		return fail("Hub store, Project or device identity missing")
	}
	session, err := s.sessions.ActiveSessionForProject(ctx, projectID)
	if err != nil || session == nil {
		return fail("active Hub session unavailable")
	}
	if session.ProjectID != projectID || session.ID == "" || session.RuntimeSnapshotID == "" ||
		session.Status != domain.SessionActive ||
		session.LifecycleState != domain.SessionLifecycleActive ||
		(session.Type != domain.SessionShow && session.Type != domain.SessionRehearsal) ||
		session.CurrentCueID == nil || strings.TrimSpace(*session.CurrentCueID) == "" ||
		session.LastCompletedCueID == nil || *session.LastCompletedCueID != *session.CurrentCueID ||
		session.StateTruth.ManualConfirmationRequired {
		return fail("current SHOW/REHEARSAL Cue is incomplete or not restorable")
	}
	running, err := s.sessions.HasRunningCueExecution(ctx, session.ID)
	if err != nil || running {
		return fail("Cue execution still running or execution status unavailable")
	}
	executions, err := s.sessions.ListCueExecutions(ctx, session.ID)
	if err != nil || len(executions) == 0 {
		return fail("no durable current Cue execution evidence")
	}
	latest, ok := latestCueExecution(executions)
	if !ok || latest.CueID != *session.CurrentCueID ||
		latest.Result != domain.ExecutionCompleted || latest.CompletedAt == nil ||
		latest.CompletedAt.Before(latest.StartedAt) {
		return fail("latest executed Cue does not confirm current completed Cue")
	}
	pinned, err := s.sessions.GetRuntimeSnapshot(ctx, session.RuntimeSnapshotID)
	if err != nil || pinned.Status != domain.SnapshotPublished ||
		pinned.ID != session.RuntimeSnapshotID || pinned.ProjectID != projectID ||
		len(pinned.Manifest) == 0 {
		return fail("current immutable published Runtime Snapshot is unavailable")
	}
	hash := sha256.Sum256(pinned.Manifest)
	if !strings.EqualFold(hex.EncodeToString(hash[:]), pinned.ContentHash) {
		return fail("published snapshot manifest hash mismatch")
	}
	manifest, err := snapshot.Decode(pinned.Manifest)
	if err != nil || manifest.SchemaVersion != snapshot.ManifestSchemaVersion ||
		manifest.ProjectID != projectID || manifest.RevisionID != pinned.RevisionID {
		return fail("manifest does not match published Project and versioned snapshot")
	}
	projection, err := DeriveCueLightingProjection(manifest, *session.CurrentCueID, deviceID)
	if err != nil {
		return fail(err.Error())
	}
	// Double-check after derivation: GO may change the current Cue while the
	// read-only snapshot is decoded. This is not a dispatch CAS; the eventual
	// correction coordinator must independently recheck immediately before
	// any command and once its ACK is received.
	latestSession, err := s.sessions.GetSession(ctx, session.ID)
	if err != nil || !sameCompletedSession(*session, latestSession) {
		return fail("Hub session or current Cue changed during desired-state read")
	}
	running, err = s.sessions.HasRunningCueExecution(ctx, session.ID)
	if err != nil || running {
		return fail("a new Cue began while deriving state")
	}
	latestExecutions, err := s.sessions.ListCueExecutions(ctx, session.ID)
	if err != nil {
		return fail("execution history unavailable during final scope check")
	}
	confirmed, ok := latestCueExecution(latestExecutions)
	if !ok || confirmed.ID != latest.ID ||
		confirmed.Result != domain.ExecutionCompleted ||
		confirmed.CompletedAt == nil {
		return fail("latest Cue execution advanced during desired-state read")
	}
	return DesiredLighting{
		ProjectID: projectID, SessionID: session.ID, SnapshotID: pinned.ID,
		SnapshotHash: pinned.ContentHash, CueID: *session.CurrentCueID,
		CueExecutionID: latest.ID, Mode: projection.Mode,
		Channels: projection.Channels, Targets: projection.Targets,
	}, nil
}

func latestCueExecution(all []domain.CueExecution) (domain.CueExecution, bool) {
	var latest domain.CueExecution
	found := false
	for _, execution := range all {
		if execution.ID == "" || execution.StartedAt.IsZero() {
			return domain.CueExecution{}, false
		}
		if !found || execution.StartedAt.After(latest.StartedAt) {
			latest, found = execution, true
		} else if execution.StartedAt.Equal(latest.StartedAt) {
			// Ambiguous event ordering; do not guess which completion is current.
			return domain.CueExecution{}, false
		}
	}
	return latest, found
}

func sameCompletedSession(old, next domain.Session) bool {
	return old.ID == next.ID && old.ProjectID == next.ProjectID &&
		old.RuntimeSnapshotID == next.RuntimeSnapshotID &&
		old.Status == next.Status && old.Status == domain.SessionActive &&
		old.LifecycleState == next.LifecycleState &&
		old.LifecycleState == domain.SessionLifecycleActive &&
		old.Type == next.Type &&
		old.CurrentCueID != nil && next.CurrentCueID != nil &&
		*old.CurrentCueID == *next.CurrentCueID &&
		old.LastCompletedCueID != nil && next.LastCompletedCueID != nil &&
		*old.LastCompletedCueID == *next.LastCompletedCueID &&
		!next.StateTruth.ManualConfirmationRequired
}

// DeriveCueLighting is the compatibility view used by existing read-only
// comparison callers. The richer projection below is authoritative for any
// future correction materialization.
func DeriveCueLighting(manifest snapshot.Manifest, cueID, deviceID string) (map[int]uint8, error) {
	projection, err := DeriveCueLightingProjection(manifest, cueID, deviceID)
	if err != nil {
		return nil, err
	}
	return copyDMXChannels(projection.Channels), nil
}

// DeriveCueLightingProjection is a pure conservative projection of the current
// Cue. It does not consult prior GO commands or invent defaults for slots
// omitted from this Cue. A partial Cue is UNKNOWN, never optimistically MATCH.
//
// CHANNELS_SET and completed CHANNELS_FADE actions preserve the exact logical
// target percentages. Historical fade timing is intentionally not preserved:
// future reconciliation materializes current state, never replays the fade.
// BLACKOUT is represented separately because physical DMX zero is not safely
// reversible to logical channel level for inverted channels.
func DeriveCueLightingProjection(
	manifest snapshot.Manifest,
	cueID, deviceID string,
) (CueLightingProjection, error) {
	fail := func(reason string) (CueLightingProjection, error) {
		return CueLightingProjection{}, fmt.Errorf("%w: %s", ErrDesiredUncertain, reason)
	}
	if manifest.SchemaVersion != snapshot.ManifestSchemaVersion ||
		strings.TrimSpace(manifest.ProjectID) == "" ||
		strings.TrimSpace(cueID) == "" || strings.TrimSpace(deviceID) == "" {
		return fail("invalid immutable manifest or Cue/device scope")
	}
	var binding *lightingnode.ProjectBinding
	for i := range manifest.LightingNodes {
		if manifest.LightingNodes[i].DeviceID == deviceID {
			if binding != nil {
				return fail("device has duplicate snapshot lighting bindings")
			}
			binding = &manifest.LightingNodes[i]
		}
	}
	if binding == nil || lightingnode.ValidateProjectBindings(manifest.LightingNodes) != nil {
		return fail("device lighting binding is missing or invalid")
	}
	configured := make(map[int]lightingnode.ChannelConfig)
	for _, channel := range binding.Configuration.Channels {
		if channel.Enabled && channel.Kind != lightingnode.ChannelUnused {
			configured[channel.ChannelNumber] = channel
		}
	}
	if len(configured) == 0 {
		return fail("device has no enabled lighting channels")
	}
	index := lightingnode.ChannelIndex(binding.Configuration)

	var cue *snapshot.Cue
	for i := range manifest.Cues {
		if manifest.Cues[i].ID == cueID {
			if cue != nil {
				return fail("ambiguous duplicate Cue ID")
			}
			cue = &manifest.Cues[i]
		}
	}
	if cue == nil || !cue.Enabled {
		return fail("current Cue not found or disabled in snapshot")
	}

	result := make(map[int]uint8)
	targets := make(map[int]DesiredLightingTarget)
	mode := DesiredLightingChannelLevels
	blackout := false
	for _, action := range cue.Actions {
		if !action.Enabled {
			continue
		}
		target := manifest.ResolveTarget(action.TargetRef)
		if target == nil {
			switch action.CapabilityKey {
			case lightingnode.CapabilityChannelsSet, lightingnode.CapabilityChannelsFade,
				lightingnode.CapabilityBlackout:
				return fail("lighting action has no published target binding")
			}
			continue
		}
		if target.LogicalType != "stage_device" {
			continue
		}
		var address struct {
			DeviceID string `json:"device_id"`
		}
		if err := json.Unmarshal(target.Configuration, &address); err != nil {
			return fail("invalid Stage Device target configuration")
		}
		if strings.TrimSpace(address.DeviceID) != deviceID {
			continue
		}
		var onError struct {
			OnError string `json:"on_error"`
		}
		if len(action.ErrorPolicy) != 0 {
			if err := json.Unmarshal(action.ErrorPolicy, &onError); err != nil {
				return fail("invalid lighting action error policy")
			}
		}
		if strings.EqualFold(strings.TrimSpace(onError.OnError), "CONTINUE") {
			return fail("lighting action may have failed without failing the Cue")
		}
		if action.ExecutionMode != "" &&
			action.ExecutionMode != "SEQUENTIAL" &&
			action.ExecutionMode != "PARALLEL_BARRIER" {
			return fail("non-deterministic lighting action execution mode")
		}

		command := ""
		switch action.CapabilityKey {
		case lightingnode.CapabilityChannelsSet:
			command = lightingnode.CommandChannelsSet
		case lightingnode.CapabilityChannelsFade:
			command = lightingnode.CommandChannelsFade
		case lightingnode.CapabilityBlackout:
			command = lightingnode.CommandBlackout
		default:
			return fail("Cue includes unsupported non-idempotent lighting action")
		}
		payload, err := lightingnode.ResolveCueCommandPayload(
			manifest.LightingNodes, deviceID, command, action.Parameters,
		)
		if err != nil {
			return fail("Cue lighting alias resolution failed: " + err.Error())
		}

		if command == lightingnode.CommandBlackout {
			if len(result) != 0 || blackout {
				return fail("combined blackout and per-channel actions are ambiguous")
			}
			blackout = true
			mode = DesiredLightingBlackout
			for slot, channel := range configured {
				result[slot] = 0 // physical failsafe zero; ignores inversion
				targets[slot] = DesiredLightingTarget{
					ChannelKey: channel.ChannelKey,
					DMXValue:   0,
				}
			}
			continue
		}
		if blackout {
			return fail("Cue cannot mix blackout with other lighting output")
		}

		var normalized map[string]float64
		if command == lightingnode.CommandChannelsSet {
			var set lightingnode.ChannelsSetPayload
			if err := json.Unmarshal(payload, &set); err != nil {
				return fail("invalid resolved lighting set")
			}
			normalized = set.Channels
		} else {
			var fade lightingnode.ChannelsFadePayload
			if err := json.Unmarshal(payload, &fade); err != nil || fade.FadeMS <= 0 {
				return fail("invalid resolved lighting fade")
			}
			normalized = fade.Channels
		}
		for key, value := range normalized {
			channel, exists := index[key]
			if !exists || !channel.Enabled || channel.Kind == lightingnode.ChannelUnused {
				return fail("resolved channel not present in bound device")
			}
			if _, duplicate := result[channel.ChannelNumber]; duplicate {
				return fail("multiple actions overwrite the same DMX slot")
			}
			dmxValue, err := lightingnode.LevelToDMX(channel, value)
			if err != nil {
				return fail("invalid pinned channel conversion")
			}
			result[channel.ChannelNumber] = dmxValue
			targets[channel.ChannelNumber] = DesiredLightingTarget{
				ChannelKey:   channel.ChannelKey,
				LogicalLevel: value,
				DMXValue:     dmxValue,
			}
		}
	}
	if len(result) != len(configured) || len(targets) != len(configured) {
		return fail("current Cue does not fully define all enabled channels; prior GO/override remains unknown")
	}
	for slot := range configured {
		if _, found := result[slot]; !found {
			return fail("one or more enabled channels have no current Cue value")
		}
		if _, found := targets[slot]; !found {
			return fail("one or more enabled channels have no materializable current Cue target")
		}
	}

	return CueLightingProjection{
		Mode:     mode,
		Channels: copyDMXChannels(result),
		Targets:  copyDesiredTargets(targets),
	}, nil
}

func copyDMXChannels(in map[int]uint8) map[int]uint8 {
	out := make(map[int]uint8, len(in))
	for slot, value := range in {
		out[slot] = value
	}
	return out
}

func copyDesiredTargets(in map[int]DesiredLightingTarget) map[int]DesiredLightingTarget {
	out := make(map[int]DesiredLightingTarget, len(in))
	for slot, target := range in {
		out[slot] = target
	}
	return out
}

// SortedChannels helps read-only UI display differences deterministically.
func SortedChannels(levels map[int]uint8) []int {
	slots := make([]int, 0, len(levels))
	for slot := range levels {
		slots = append(slots, slot)
	}
	sort.Ints(slots)
	return slots
}

