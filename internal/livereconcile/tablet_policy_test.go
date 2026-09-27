package livereconcile

import (
	"reflect"
	"testing"
)

func strptr(value string) *string { return &value }
func boolptr(value bool) *bool    { return &value }
func intptr(value int) *int       { return &value }

func TestTabletReconnectStatefulDisplayFieldsAreCandidates(t *testing.T) {
	desired := TabletPlayerState{
		PreparedMainMedia:   strptr("scene-b.mp4"),
		LiveSource:          strptr("http://relay:9081/api/v0/stream"),
		VideoScaleMode:      strptr("CROP"),
		LiveRotationDegrees: intptr(90),
		Blackout:            boolptr(true),
	}
	observed := TabletPlayerState{
		PreparedMainMedia:   strptr("scene-a.mp4"),
		LiveSource:          strptr(""),
		VideoScaleMode:      strptr("FIT"),
		LiveRotationDegrees: intptr(0),
		Blackout:            boolptr(false),
	}
	result := CompareTabletReconnectState(desired, observed)
	want := []string{
		"blackout",
		"live_rotation_degrees",
		"live_source",
		"prepared_main_media",
		"video_scale_mode",
	}
	if !reflect.DeepEqual(result.StatefulCandidates, want) ||
		len(result.ManualFields) != 0 || len(result.UnknownFields) != 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestTabletReconnectDoesNotReplayDifferentMainMedia(t *testing.T) {
	result := CompareTabletReconnectState(
		TabletPlayerState{
			MainMedia:   strptr("scene-b.mp4"),
			MainPlaying: boolptr(true),
		},
		TabletPlayerState{
			MainMedia:   strptr("scene-a.mp4"),
			MainPlaying: boolptr(false),
		},
	)
	if !reflect.DeepEqual(result.ManualFields, []string{"main_media", "main_playing"}) ||
		len(result.StatefulCandidates) != 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestTabletReconnectDoesNotReplayOverlayButCanClearIt(t *testing.T) {
	replay := CompareTabletReconnectState(
		TabletPlayerState{OverlayMedia: strptr("overlay-b.mp4")},
		TabletPlayerState{OverlayMedia: strptr("overlay-a.mp4")},
	)
	if !reflect.DeepEqual(replay.ManualFields, []string{"overlay_media"}) {
		t.Fatalf("replay=%+v", replay)
	}

	clear := CompareTabletReconnectState(
		TabletPlayerState{OverlayMedia: strptr("")},
		TabletPlayerState{OverlayMedia: strptr("overlay-a.mp4")},
	)
	if !reflect.DeepEqual(clear.StatefulCandidates, []string{"overlay_media"}) {
		t.Fatalf("clear=%+v", clear)
	}
}

func TestTabletReconnectCanPauseSameMediaButCannotResumeWithoutTimelinePolicy(t *testing.T) {
	pause := CompareTabletReconnectState(
		TabletPlayerState{
			MainMedia: strptr("scene-a.mp4"), MainPlaying: boolptr(false),
		},
		TabletPlayerState{
			MainMedia: strptr("scene-a.mp4"), MainPlaying: boolptr(true),
		},
	)
	if !reflect.DeepEqual(pause.StatefulCandidates, []string{"main_playing"}) {
		t.Fatalf("pause=%+v", pause)
	}

	resume := CompareTabletReconnectState(
		TabletPlayerState{
			MainMedia: strptr("scene-a.mp4"), MainPlaying: boolptr(true),
		},
		TabletPlayerState{
			MainMedia: strptr("scene-a.mp4"), MainPlaying: boolptr(false),
		},
	)
	if !reflect.DeepEqual(resume.ManualFields, []string{"main_playing"}) {
		t.Fatalf("resume=%+v", resume)
	}
}

func TestTabletReconnectBlackoutEntryIsSafeButClearRequiresConfirmation(t *testing.T) {
	enter := CompareTabletReconnectState(
		TabletPlayerState{Blackout: boolptr(true)},
		TabletPlayerState{Blackout: boolptr(false)},
	)
	if !reflect.DeepEqual(enter.StatefulCandidates, []string{"blackout"}) {
		t.Fatalf("enter=%+v", enter)
	}

	clear := CompareTabletReconnectState(
		TabletPlayerState{Blackout: boolptr(false)},
		TabletPlayerState{Blackout: boolptr(true)},
	)
	if !reflect.DeepEqual(clear.ManualFields, []string{"blackout"}) {
		t.Fatalf("clear=%+v", clear)
	}
}

func TestTabletReconnectMissingFreshFieldIsUnknown(t *testing.T) {
	result := CompareTabletReconnectState(
		TabletPlayerState{
			LiveSource: strptr("http://relay:9081/api/v0/stream"),
			Blackout:   boolptr(true),
		},
		TabletPlayerState{},
	)
	if !reflect.DeepEqual(result.UnknownFields, []string{"blackout", "live_source"}) ||
		len(result.StatefulCandidates) != 0 || len(result.ManualFields) != 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestTabletReconnectMatchingFieldsProduceNoAction(t *testing.T) {
	desired := TabletPlayerState{
		MainMedia:           strptr("scene-a.mp4"),
		PreparedMainMedia:   strptr("scene-b.mp4"),
		MainPlaying:         boolptr(false),
		OverlayMedia:        strptr(""),
		LiveSource:          strptr(""),
		Blackout:            boolptr(true),
		VideoScaleMode:      strptr("FIT"),
		LiveRotationDegrees: intptr(0),
	}
	result := CompareTabletReconnectState(desired, desired)
	if len(result.StatefulCandidates) != 0 ||
		len(result.ManualFields) != 0 ||
		len(result.UnknownFields) != 0 ||
		len(result.Fields) != 8 {
		t.Fatalf("result=%+v", result)
	}
	for _, field := range result.Fields {
		if field.Status != TabletFieldMatch {
			t.Fatalf("field=%+v", field)
		}
	}
}
