package cueengine

import (
	"encoding/json"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/ali96adil/StageCore/internal/snapshot"
)

// outputResourceKeys conservatively identifies external output ownership from
// the *published* snapshot. An alias label is not a physical device identity:
// multiple project aliases may point at the same Stage Device, Companion or
// OSC endpoint. Unknown mappings use "*" and cannot overlap with any other
// external output; they must not silently fall back to per-alias arbitration.
func outputResourceKeys(manifest snapshot.Manifest, action snapshot.Action) []string {
	ref := strings.TrimSpace(action.TargetRef)
	if ref == "" {
		return []string{"*"}
	}
	target := manifest.ResolveTarget(ref)
	if target == nil {
		return []string{"*"}
	}
	seen := map[string]struct{}{"alias:" + ref: {}}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(target.Configuration, &config); err != nil {
		return []string{"*"}
	}

	resolved := false
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[kind+":"+value] = struct{}{}
			resolved = true
		}
	}
	str := func(obj map[string]json.RawMessage, key string) string {
		var value string
		if err := json.Unmarshal(obj[key], &value); err != nil {
			return ""
		}
		return value
	}

	for _, field := range []string{
		"device_id", "stage_device_id", "tablet_id", "companion_id",
		"machine_id", "target_device_id",
	} {
		add("device", str(config, field))
	}
	for _, field := range []string{"machine_role_id", "role_id"} {
		add("role", str(config, field))
	}
	for _, field := range []string{
		"endpoint_id", "destination_id", "osc_destination_id",
		"midi_destination_id",
	} {
		add("destination", str(config, field))
	}
	var deviceIDs []string
	if raw, ok := config["device_ids"]; ok {
		if err := json.Unmarshal(raw, &deviceIDs); err != nil || len(deviceIDs) == 0 {
			return []string{"*"}
		}
		for _, id := range deviceIDs {
			if strings.TrimSpace(id) == "" {
				return []string{"*"}
			}
			add("device", id)
		}
	}
	// Both OSC aliases may specify the exact same UDP endpoint inside a nested
	// `osc` config. Normalized host:port is a shared output even when the
	// aliases differ (for example, VDMX MAIN / VDMX TEST).
	osc := config
	if raw, ok := config["osc"]; ok {
		if err := json.Unmarshal(raw, &osc); err != nil {
			return []string{"*"}
		}
	}
	host := strings.ToLower(strings.TrimSpace(str(osc, "host")))
	if host != "" {
		var port int
		if raw, ok := osc["port"]; ok {
			if err := json.Unmarshal(raw, &port); err != nil {
				return []string{"*"}
			}
		}
		if port < 1 || port > 65535 {
			return []string{"*"}
		}
		// DNS/mDNS aliases may resolve to an address already represented
		// by another OSC alias. Since the snapshot cannot prove runtime DNS
		// equivalence, refuse physical concurrency for non-literal hosts.
		parsed := net.ParseIP(host)
		if parsed == nil {
			return []string{"*"}
		}
		host = parsed.String()
		add("osc", net.JoinHostPort(host, strconv.Itoa(port)))
	} else if _, declared := osc["port"]; declared {
		return []string{"*"}
	}

	if !resolved {
		// No validated physical identity is available. It would be unsafe
		// to assume two unqualified aliases drive different devices.
		return []string{"*"}
	}
	result := make([]string, 0, len(seen))
	for key := range seen {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
