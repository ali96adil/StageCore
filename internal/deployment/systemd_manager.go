package deployment

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type systemdCommand func(context.Context, string, ...string) ([]byte, error)

// ValidateSystemdServiceTarget verifies that systemd resolves the selected
// unit to the exact managed file before an update stops or starts it.
func ValidateSystemdServiceTarget(ctx context.Context, opts Options, run systemdCommand) error {
	if run == nil {
		return fmt.Errorf("systemd command runner is required")
	}
	if err := ValidateSystemdOperationPath(opts); err != nil {
		return err
	}
	properties := []string{
		"Id", "Names", "LoadState", "FragmentPath", "PartOf", "BindsTo",
		"Conflicts", "Requires", "Requisite", "Upholds", "PropagatesStopTo",
		"StopPropagatedFrom", "TriggeredBy", "Triggers", "BoundBy", "ConsistsOf",
		"Before", "After", "Wants", "WantedBy", "RequiredBy",
	}
	args := []string{"show", "--no-pager"}
	for _, property := range properties {
		args = append(args, "--property="+property)
	}
	args = append(args, opts.ServiceName)
	output, err := run(ctx, "systemctl", args...)
	if err != nil {
		return fmt.Errorf("inspect systemd target %s: %w", opts.ServiceName, err)
	}
	return validateSystemdShowOutput(opts, string(output))
}

func validateSystemdShowOutput(opts Options, output string) error {
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || key == "" {
			continue
		}
		if _, duplicate := values[key]; duplicate {
			return fmt.Errorf("systemd returned duplicate %s properties", key)
		}
		values[key] = strings.TrimSpace(value)
	}
	if values["LoadState"] != "loaded" {
		return fmt.Errorf("systemd target %s is not loaded", opts.ServiceName)
	}
	if values["Id"] != opts.ServiceName {
		return fmt.Errorf("systemd resolved %s as %q", opts.ServiceName, values["Id"])
	}
	names := strings.Fields(values["Names"])
	if len(names) != 1 || names[0] != opts.ServiceName {
		return fmt.Errorf("systemd target %s has unexpected aliases %q", opts.ServiceName, values["Names"])
	}
	if filepath.Clean(values["FragmentPath"]) != filepath.Clean(opts.SystemdUnit) {
		return fmt.Errorf("systemd target %s resolves to %q instead of %q", opts.ServiceName, values["FragmentPath"], opts.SystemdUnit)
	}
	for _, property := range []string{
		"PartOf", "BindsTo", "Conflicts", "Requires", "Requisite", "Upholds",
		"PropagatesStopTo", "StopPropagatedFrom", "TriggeredBy", "Triggers",
		"BoundBy", "ConsistsOf", "Before", "After", "Wants", "WantedBy", "RequiredBy",
	} {
		for _, unit := range strings.Fields(values[property]) {
			if unit == DefaultServiceName || unit == "stagecore-camera-relay.service" {
				return fmt.Errorf("systemd target %s is linked to protected service %s", opts.ServiceName, unit)
			}
		}
	}
	return nil
}
