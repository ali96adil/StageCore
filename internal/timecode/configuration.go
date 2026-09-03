package timecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/snapshot"
)

const SourceTargetLogicalType = "TIMECODE_SOURCE"

type ManifestConfiguration struct {
	Enabled    bool            `json:"enabled"`
	TargetRef  string          `json:"target_ref,omitempty"`
	Source     SourceSelection `json:"source,omitempty"`
	StartFrame int64           `json:"start_frame,omitempty"`
	Bindings   []Binding       `json:"bindings,omitempty"`
}

type sourceTargetConfiguration struct {
	SourceID     string     `json:"source_id"`
	Kind         SourceKind `json:"kind"`
	Rate         string     `json:"rate"`
	OffsetFrames int64      `json:"offset_frames"`
	Start        string     `json:"start_timecode"`
}

type cueExecutionPolicy struct {
	Timecode *struct {
		BindingID   string `json:"binding_id"`
		At          string `json:"at"`
		ExpiryFrames int64 `json:"expiry_frames"`
		Enabled     *bool  `json:"enabled"`
	} `json:"timecode"`
}

func ResolveManifestConfiguration(manifest snapshot.Manifest) (ManifestConfiguration, error) {
	var target *snapshot.Target
	for i := range manifest.Targets {
		candidate := manifest.Targets[i]
		if !strings.EqualFold(strings.TrimSpace(candidate.LogicalType), SourceTargetLogicalType) {
			continue
		}
		if target != nil {
			return ManifestConfiguration{}, errors.New("runtime snapshot contains multiple TIMECODE_SOURCE targets")
		}
		target = &candidate
	}

	bindingsPresent := false
	for _, cue := range manifest.Cues {
		policy, present, err := decodeCueTimecodePolicy(cue)
		if err != nil {
			return ManifestConfiguration{}, err
		}
		if present && policy.Timecode != nil {
			bindingsPresent = true
			break
		}
	}
	if target == nil {
		if bindingsPresent {
			return ManifestConfiguration{}, errors.New("timecode cue bindings require exactly one TIMECODE_SOURCE target")
		}
		return ManifestConfiguration{Enabled: false}, nil
	}

	var raw sourceTargetConfiguration
	if err := json.Unmarshal(target.Configuration, &raw); err != nil {
		return ManifestConfiguration{}, fmt.Errorf("decode timecode source %s: %w", target.TargetRef, err)
	}
	raw.SourceID = strings.TrimSpace(raw.SourceID)
	if raw.SourceID == "" {
		raw.SourceID = strings.TrimSpace(target.TargetRef)
	}
	if raw.SourceID == "" {
		return ManifestConfiguration{}, errors.New("timecode source_id is required")
	}
	raw.Kind = SourceKind(strings.ToUpper(strings.TrimSpace(string(raw.Kind))))
	switch raw.Kind {
	case SourceInternal, SourceMTC, SourceLTC:
	default:
		return ManifestConfiguration{}, fmt.Errorf("unsupported timecode source kind %q", raw.Kind)
	}
	rate, err := ParseRate(raw.Rate)
	if err != nil {
		return ManifestConfiguration{}, err
	}
	startFrame := int64(0)
	if strings.TrimSpace(raw.Start) != "" {
		start, err := Parse(raw.Start, rate)
		if err != nil {
			return ManifestConfiguration{}, fmt.Errorf("parse start_timecode: %w", err)
		}
		startFrame, err = start.FrameNumber()
		if err != nil {
			return ManifestConfiguration{}, err
		}
	}
	cfg := ManifestConfiguration{
		Enabled: true,
		TargetRef: target.TargetRef,
		Source: SourceSelection{SourceID: raw.SourceID, Kind: raw.Kind, Rate: rate, OffsetFrames: raw.OffsetFrames},
		StartFrame: startFrame,
		Bindings: []Binding{},
	}

	seen := map[string]struct{}{}
	for _, cue := range manifest.Cues {
		policy, present, err := decodeCueTimecodePolicy(cue)
		if err != nil {
			return ManifestConfiguration{}, err
		}
		if !present || policy.Timecode == nil {
			continue
		}
		bindingEnabled := true
		if policy.Timecode.Enabled != nil {
			bindingEnabled = *policy.Timecode.Enabled
		}
		bindingID := strings.TrimSpace(policy.Timecode.BindingID)
		if bindingID == "" {
			bindingID = "cue:" + cue.ID
		}
		if _, exists := seen[bindingID]; exists {
			return ManifestConfiguration{}, fmt.Errorf("duplicate timecode binding_id %q", bindingID)
		}
		seen[bindingID] = struct{}{}
		at := strings.TrimSpace(policy.Timecode.At)
		if at == "" {
			return ManifestConfiguration{}, fmt.Errorf("timecode binding %s requires at", bindingID)
		}
		tc, err := Parse(at, rate)
		if err != nil {
			return ManifestConfiguration{}, fmt.Errorf("timecode binding %s: %w", bindingID, err)
		}
		targetFrame, err := tc.FrameNumber()
		if err != nil {
			return ManifestConfiguration{}, err
		}
		if policy.Timecode.ExpiryFrames < 0 {
			return ManifestConfiguration{}, fmt.Errorf("timecode binding %s expiry_frames must be >= 0", bindingID)
		}
		cfg.Bindings = append(cfg.Bindings, Binding{
			BindingID: bindingID,
			CueID: cue.ID,
			TargetFrame: targetFrame,
			ExpiryFrames: policy.Timecode.ExpiryFrames,
			Enabled: bindingEnabled && cue.Enabled,
		})
	}
	sort.Slice(cfg.Bindings, func(i, j int) bool {
		if cfg.Bindings[i].TargetFrame == cfg.Bindings[j].TargetFrame {
			return cfg.Bindings[i].BindingID < cfg.Bindings[j].BindingID
		}
		return cfg.Bindings[i].TargetFrame < cfg.Bindings[j].TargetFrame
	})
	return cfg, nil
}

func decodeCueTimecodePolicy(cue snapshot.Cue) (cueExecutionPolicy, bool, error) {
	if len(cue.ExecutionPolicy) == 0 || string(cue.ExecutionPolicy) == "{}" || string(cue.ExecutionPolicy) == "null" {
		return cueExecutionPolicy{}, false, nil
	}
	var policy cueExecutionPolicy
	if err := json.Unmarshal(cue.ExecutionPolicy, &policy); err != nil {
		return cueExecutionPolicy{}, false, fmt.Errorf("decode cue %s execution_policy: %w", cue.ID, err)
	}
	return policy, policy.Timecode != nil, nil
}

func (c ManifestConfiguration) NormalizeRawFrame(rawFrame int64, observedAtUnixNano int64, driftFrames int64, discontinuity bool) (Sample, error) {
	if !c.Enabled {
		return Sample{}, errors.New("timecode is disabled in runtime snapshot")
	}
	frame, err := ApplyOffset(rawFrame, c.Source.OffsetFrames)
	if err != nil {
		return Sample{}, err
	}
	tc, err := FromFrameNumber(frame, c.Source.Rate)
	if err != nil {
		return Sample{}, err
	}
	return Sample{
		SourceID: c.Source.SourceID,
		Kind: c.Source.Kind,
		Rate: c.Source.Rate,
		Timecode: tc,
		FrameNumber: frame,
		RawFrame: rawFrame,
		OffsetFrames: c.Source.OffsetFrames,
		ObservedAt: unixNanoUTC(observedAtUnixNano),
		Transport: TransportRunning,
		Discontinuity: discontinuity,
		DriftFrames: driftFrames,
	}, nil
}
