package cueengine

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/ali96adil/StageCore/internal/snapshot"
)

func snapshotAlias(ref, config string) snapshot.Target {
	return snapshot.Target{
		TargetRef: ref, LogicalType: "STAGE_DEVICE",
		Configuration: json.RawMessage(config),
	}
}

func TestOverlapResourcesCanonicalizeDifferentAliasesForSamePhysicalDevice(t *testing.T) {
	manifest := snapshot.Manifest{
		Targets: []snapshot.Target{
			snapshotAlias("front_cold", `{"device_id":"lighting-node-01"}`),
			snapshotAlias("front_warm", `{"device_id":"lighting-node-01"}`),
			snapshotAlias("back_warm", `{"device_id":"lighting-node-02"}`),
		},
	}
	first := outputResourceKeys(manifest, snapshot.Action{TargetRef: "front_cold"})
	second := outputResourceKeys(manifest, snapshot.Action{TargetRef: "front_warm"})
	independent := outputResourceKeys(manifest, snapshot.Action{TargetRef: "back_warm"})
	if !slices.Contains(first, "device:lighting-node-01") ||
		!slices.Contains(second, "device:lighting-node-01") {
		t.Fatalf("two aliases for same DMX node did not preserve canonical identity: %v / %v", first, second)
	}
	if slices.Contains(independent, "device:lighting-node-01") {
		t.Fatalf("different lighting device has a false identity overlap: %v", independent)
	}
}

func TestOverlapResourcesCanonicalizeOSCAndBroadcast(t *testing.T) {
	manifest := snapshot.Manifest{Targets: []snapshot.Target{
		snapshotAlias("video-main", `{"osc":{"host":"127.0.0.1","port":3546}}`),
		snapshotAlias("video-test", `{"osc":{"host":"127.0.0.1","port":3546}}`),
		snapshotAlias("four-tablets", `{"device_ids":["tablet-a","tablet-b"]}`),
		snapshotAlias("one-tablet", `{"device_id":"tablet-b"}`),
	}}
	for _, ref := range []string{"video-main","video-test"} {
		if resources := outputResourceKeys(manifest, snapshot.Action{TargetRef: ref});
			!slices.Contains(resources, "osc:127.0.0.1:3546") {
			t.Fatalf("OSC endpoint not recognized for %s: %v", ref, resources)
		}
	}
	broadcast := outputResourceKeys(manifest, snapshot.Action{TargetRef: "four-tablets"})
	single := outputResourceKeys(manifest, snapshot.Action{TargetRef: "one-tablet"})
	if !slices.Contains(broadcast, "device:tablet-a") ||
		!slices.Contains(broadcast, "device:tablet-b") ||
		!slices.Contains(single, "device:tablet-b") {
		t.Fatalf("broadcast target loses physical device identity: %v / %v", broadcast, single)
	}
}

func TestOverlapResourcesFailClosedForUnknownAliasOrMalformedConfiguration(t *testing.T) {
	manifest := snapshot.Manifest{Targets: []snapshot.Target{
		snapshotAlias("missing-metadata", `{}`),
		snapshotAlias("invalid-osc-port", `{"osc":{"host":"127.0.0.1","port":0}}`),
		snapshotAlias("dns-alias", `{"osc":{"host":"my-macbook.local","port":3546}}`),
		snapshotAlias("malformed-devices", `{"device_ids":["tablet-1",null]}`),
	}}
	for _, ref := range []string{
		"nonexistent", "", "missing-metadata", "invalid-osc-port",
		"dns-alias", "malformed-devices",
	} {
		got := outputResourceKeys(manifest, snapshot.Action{TargetRef: ref})
		if !slices.Equal(got, []string{"*"}) {
			t.Fatalf("unresolved output alias %q must fence all outputs, got %v", ref, got)
		}
	}
}

func TestOverlapResourcesIncludeActionSelectedTabletRecipients(t *testing.T) {
    manifest := snapshot.Manifest{Targets: []snapshot.Target{
        snapshotAlias("tablet-controller-one", `{"device_id":"controller-a"}`),
        snapshotAlias("tablet-controller-two", `{"device_id":"controller-b"}`),
    }}
    group := outputResourceKeys(manifest,snapshot.Action{
        TargetRef:"tablet-controller-one",
        Parameters:json.RawMessage(`{"device_ids":["tablet-a","tablet-b"],"mode":"PLAY"}`),
    })
    direct := outputResourceKeys(manifest,snapshot.Action{
        TargetRef:"tablet-controller-two",
        Parameters:json.RawMessage(`{"tablet_id":"tablet-b","mode":"BLACKOUT"}`),
    })
    if !slices.Contains(group,"device:tablet-b") || !slices.Contains(direct,"device:tablet-b") {
        t.Fatalf("physical tablet overlap hidden by distinct target aliases: group=%v direct=%v",group,direct)
    }
    for _, bad := range []json.RawMessage{
        json.RawMessage(`{"device_ids":[]}`),
        json.RawMessage(`{"device_ids":["tablet-a",null]}`),
        json.RawMessage(`{"tablet_id":""}`),
        json.RawMessage(`{"stage_device_id":42}`),
        json.RawMessage(`{"tablet_device_ids":[""]}`),
        json.RawMessage(`not-json`),
    } {
        got:=outputResourceKeys(manifest,snapshot.Action{TargetRef:"tablet-controller-one",Parameters:bad})
        if !slices.Equal(got,[]string{"*"}) {
            t.Fatalf("malformed action-selected recipient must block all overlaps: params=%s got=%v",bad,got)
        }
    }
}
