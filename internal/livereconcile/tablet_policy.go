package livereconcile

import "sort"

type TabletFieldStatus string

const (
	TabletFieldMatch             TabletFieldStatus = "MATCH"
	TabletFieldStatefulCandidate TabletFieldStatus = "STATEFUL_CANDIDATE"
	TabletFieldManual            TabletFieldStatus = "MANUAL_CONFIRMATION"
	TabletFieldUnknown           TabletFieldStatus = "UNKNOWN"
)

// TabletPlayerState mirrors only the typed current player surface reported by
// the authenticated Tablet Player. nil means the field is not governed (for a
// desired state) or was not freshly observed (for an observation).
type TabletPlayerState struct {
	MainMedia           *string
	PreparedMainMedia   *string
	MainPlaying         *bool
	OverlayMedia        *string
	LiveSource          *string
	Blackout            *bool
	VideoScaleMode      *string
	LiveRotationDegrees *int
}

type TabletFieldComparison struct {
	Field  string
	Status TabletFieldStatus
	Reason string
}

type TabletReconnectPolicyResult struct {
	Fields             []TabletFieldComparison
	StatefulCandidates []string
	ManualFields       []string
	UnknownFields      []string
}

// CompareTabletReconnectState is a pure, no-command policy. It separates
// stateful/idempotent corrections from operations that could replay timed
// media. It has no Project/session/socket authority and MUST be used only after
// the caller independently validates current Hub scope and fresh observation.
func CompareTabletReconnectState(
	desired TabletPlayerState,
	observed TabletPlayerState,
) TabletReconnectPolicyResult {
	var out TabletReconnectPolicyResult

	add := func(field string, status TabletFieldStatus, reason string) {
		out.Fields = append(out.Fields, TabletFieldComparison{
			Field: field, Status: status, Reason: reason,
		})
		switch status {
		case TabletFieldStatefulCandidate:
			out.StatefulCandidates = append(out.StatefulCandidates, field)
		case TabletFieldManual:
			out.ManualFields = append(out.ManualFields, field)
		case TabletFieldUnknown:
			out.UnknownFields = append(out.UnknownFields, field)
		}
	}

	compareString := func(
		field string,
		want, got *string,
		classify func(want, got string) (TabletFieldStatus, string),
	) {
		if want == nil {
			return
		}
		if got == nil {
			add(field, TabletFieldUnknown, "field was not freshly observed")
			return
		}
		if *want == *got {
			add(field, TabletFieldMatch, "fresh observed state matches")
			return
		}
		status, reason := classify(*want, *got)
		add(field, status, reason)
	}

	compareString(
		"prepared_main_media",
		desired.PreparedMainMedia,
		observed.PreparedMainMedia,
		func(_, _ string) (TabletFieldStatus, string) {
			return TabletFieldStatefulCandidate,
				"preparing media is bounded and does not start historical playback"
		},
	)

	compareString(
		"live_source",
		desired.LiveSource,
		observed.LiveSource,
		func(_, _ string) (TabletFieldStatus, string) {
			return TabletFieldStatefulCandidate,
				"Live source visibility is current state and has no historical timeline to replay"
		},
	)

	compareString(
		"video_scale_mode",
		desired.VideoScaleMode,
		observed.VideoScaleMode,
		func(_, _ string) (TabletFieldStatus, string) {
			return TabletFieldStatefulCandidate,
				"scale mode is idempotent current display state"
		},
	)

	if desired.LiveRotationDegrees != nil {
		if observed.LiveRotationDegrees == nil {
			add("live_rotation_degrees", TabletFieldUnknown,
				"field was not freshly observed")
		} else if *desired.LiveRotationDegrees == *observed.LiveRotationDegrees {
			add("live_rotation_degrees", TabletFieldMatch,
				"fresh observed state matches")
		} else {
			add("live_rotation_degrees", TabletFieldStatefulCandidate,
				"rotation is idempotent current display state")
		}
	}

	compareString(
		"main_media",
		desired.MainMedia,
		observed.MainMedia,
		func(want, _ string) (TabletFieldStatus, string) {
			if want == "" {
				return TabletFieldStatefulCandidate,
					"removing/stopping visible main media does not replay an old clip"
			}
			return TabletFieldManual,
				"changing main media could restart timed content; playback position is not proven"
		},
	)

	compareString(
		"overlay_media",
		desired.OverlayMedia,
		observed.OverlayMedia,
		func(want, _ string) (TabletFieldStatus, string) {
			if want == "" {
				return TabletFieldStatefulCandidate,
					"clearing an overlay does not replay historical overlay content"
			}
			return TabletFieldManual,
				"showing a different overlay could replay timed content"
		},
	)

	if desired.MainPlaying != nil {
		if observed.MainPlaying == nil {
			add("main_playing", TabletFieldUnknown,
				"playback flag was not freshly observed")
		} else if *desired.MainPlaying == *observed.MainPlaying {
			add("main_playing", TabletFieldMatch, "fresh observed state matches")
		} else if !*desired.MainPlaying && sameStringValue(
			desired.MainMedia, observed.MainMedia,
		) {
			add("main_playing", TabletFieldStatefulCandidate,
				"pausing/stopping the same current main media cannot replay a missed Cue")
		} else {
			add("main_playing", TabletFieldManual,
				"starting/resuming playback requires timeline or explicit restart policy")
		}
	}

	if desired.Blackout != nil {
		if observed.Blackout == nil {
			add("blackout", TabletFieldUnknown, "blackout was not freshly observed")
		} else if *desired.Blackout == *observed.Blackout {
			add("blackout", TabletFieldMatch, "fresh observed state matches")
		} else if *desired.Blackout {
			add("blackout", TabletFieldStatefulCandidate,
				"entering blackout is a bounded safety-increasing state change")
		} else {
			add("blackout", TabletFieldManual,
				"clearing blackout may reveal stale media; require validated visible-surface state")
		}
	}

	sort.Strings(out.StatefulCandidates)
	sort.Strings(out.ManualFields)
	sort.Strings(out.UnknownFields)
	sort.Slice(out.Fields, func(i, j int) bool {
		return out.Fields[i].Field < out.Fields[j].Field
	})
	return out
}

func sameStringValue(a, b *string) bool {
	return a != nil && b != nil && *a == *b
}
