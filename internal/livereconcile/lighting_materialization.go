package livereconcile

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/lightingnode"
)

// LightingCorrectionMaterialization is still a source-only intent. It contains
// the exact current-state command type/payload a later coordinator could use
// after repeating every authority/race fence. It grants no dispatch authority
// and contains no historical Cue action, GO, fade duration or execution replay.
type LightingCorrectionMaterialization struct {
	CommandType          string
	Payload              json.RawMessage
	DifferingSlots       []int
	ExpectedDMX          map[int]uint8
	LogicalChannels      map[string]float64
	DispatchAllowed      bool
	RequiresRevalidation bool
}

// MaterializeLightingCorrection converts a previously fenced correction plan
// into a current-state lighting intent without ever reverse-converting a DMX
// byte to a logical percentage.
//
// Completed CHANNELS_FADE state is materialized as an immediate CHANNELS_SET
// of only the differing channels; historical fade timing is never replayed.
// BLACKOUT remains a real BLACKOUT command because physical zero cannot safely
// be represented as logical level zero for inverted channels.
func MaterializeLightingCorrection(
	plan ActiveLightingPartialPlan,
	desired DesiredLighting,
) (LightingCorrectionMaterialization, error) {
	fail := func(reason string) (LightingCorrectionMaterialization, error) {
		return LightingCorrectionMaterialization{}, fmt.Errorf(
			"lighting correction cannot be materialized: %s", reason,
		)
	}

	if plan.Status != ActivePartialPlanCandidate ||
		plan.DispatchAllowed ||
		!plan.RequiresRevalidation {
		return fail("plan is not a fenced correction candidate")
	}
	if strings.TrimSpace(plan.ProjectID) == "" ||
		plan.ProjectID != desired.ProjectID ||
		plan.SessionID != desired.SessionID ||
		plan.SnapshotID != desired.SnapshotID ||
		plan.CueID != desired.CueID ||
		plan.CueExecutionID != desired.CueExecutionID {
		return fail("plan identity does not match current desired state")
	}
	if len(plan.DifferingSlots) == 0 ||
		len(plan.TargetSlots) != len(plan.DifferingSlots) {
		return fail("plan has no bounded slot delta")
	}

	slots := append([]int(nil), plan.DifferingSlots...)
	sort.Ints(slots)
	for index := 1; index < len(slots); index++ {
		if slots[index] == slots[index-1] {
			return fail("plan contains duplicate DMX slots")
		}
	}
	if !reflect.DeepEqual(slots, plan.DifferingSlots) {
		return fail("plan DMX slots are not canonically ordered")
	}

	expected := make(map[int]uint8, len(slots))
	for _, slot := range slots {
		if slot < 1 || slot > lightingnode.MaxChannels {
			return fail("plan contains a slot outside the lighting node")
		}
		want, exists := desired.Channels[slot]
		if !exists || plan.TargetSlots[slot] != want {
			return fail("plan DMX target no longer matches current desired state")
		}
		target, exists := desired.Targets[slot]
		if !exists || target.DMXValue != want ||
			strings.TrimSpace(target.ChannelKey) == "" {
			return fail("exact logical Cue target is unavailable for a planned slot")
		}
		expected[slot] = want
	}

	switch desired.Mode {
	case DesiredLightingChannelLevels:
		logical := make(map[string]float64, len(slots))
		for _, slot := range slots {
			target := desired.Targets[slot]
			if math.IsNaN(target.LogicalLevel) ||
				math.IsInf(target.LogicalLevel, 0) ||
				target.LogicalLevel < 0 ||
				target.LogicalLevel > 100 {
				return fail("logical Cue target is invalid")
			}
			key := strings.TrimSpace(target.ChannelKey)
			if _, duplicate := logical[key]; duplicate {
				return fail("multiple planned slots map to the same logical channel")
			}
			logical[key] = target.LogicalLevel
		}
		raw, err := json.Marshal(lightingnode.ChannelsSetPayload{Channels: logical})
		if err != nil {
			return fail("logical correction payload could not be encoded")
		}
		canonical, err := lightingnode.CanonicalCommandPayload(
			lightingnode.CommandChannelsSet,
			raw,
		)
		if err != nil {
			return fail("logical correction payload is not canonical")
		}
		return LightingCorrectionMaterialization{
			CommandType:          lightingnode.CommandChannelsSet,
			Payload:              canonical,
			DifferingSlots:       append([]int(nil), slots...),
			ExpectedDMX:          copyDMXChannels(expected),
			LogicalChannels:      copyLogicalChannels(logical),
			DispatchAllowed:      false,
			RequiresRevalidation: true,
		}, nil

	case DesiredLightingBlackout:
		// A blackout command touches all output slots, so require that the
		// entire current desired projection is physical zero. This prevents a
		// malformed mixed projection from widening a partial correction.
		if len(desired.Channels) == 0 ||
			len(desired.Targets) != len(desired.Channels) {
			return fail("blackout projection is incomplete")
		}
		for slot, value := range desired.Channels {
			target, exists := desired.Targets[slot]
			if !exists || value != 0 || target.DMXValue != 0 {
				return fail("blackout projection contains a nonzero or missing slot")
			}
		}
		raw, err := json.Marshal(lightingnode.BlackoutPayload{FadeMS: 0})
		if err != nil {
			return fail("blackout payload could not be encoded")
		}
		canonical, err := lightingnode.CanonicalCommandPayload(
			lightingnode.CommandBlackout,
			raw,
		)
		if err != nil {
			return fail("blackout payload is not canonical")
		}
		return LightingCorrectionMaterialization{
			CommandType:          lightingnode.CommandBlackout,
			Payload:              canonical,
			DifferingSlots:       append([]int(nil), slots...),
			ExpectedDMX:          copyDMXChannels(expected),
			DispatchAllowed:      false,
			RequiresRevalidation: true,
		}, nil

	default:
		return fail("desired lighting mode is missing or unsupported")
	}
}

func copyLogicalChannels(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
