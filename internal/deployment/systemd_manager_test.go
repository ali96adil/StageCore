package deployment

import (
	"strings"
	"testing"
)

func TestValidateSystemdShowOutputRejectsWrongIdentityAndProductionLinks(t *testing.T) {
	opts := Options{
		ServiceName: "stagecore-rc-qualification.service",
		SystemdUnit: "/etc/systemd/system/stagecore-rc-qualification.service",
	}
	base := strings.Join([]string{
		"Id=" + opts.ServiceName,
		"Names=" + opts.ServiceName,
		"LoadState=loaded",
		"FragmentPath=" + opts.SystemdUnit,
		"PrivateNetwork=yes",
		"PrivateDevices=yes",
		"DevicePolicy=closed",
		"PartOf=",
		"Wants=network.target",
	}, "\n")
	if err := validateSystemdShowOutput(opts, base); err != nil {
		t.Fatalf("valid systemd target rejected: %v", err)
	}

	cases := []struct {
		name string
		edit func(string) string
	}{
		{"production fragment", func(input string) string {
			return strings.Replace(input, "FragmentPath="+opts.SystemdUnit, "FragmentPath="+DefaultUnitPath, 1)
		}},
		{"production alias", func(input string) string {
			return strings.Replace(input, "Names="+opts.ServiceName, "Names="+opts.ServiceName+" stagecore-hub.service", 1)
		}},
		{"production unit relationship", func(input string) string { return strings.Replace(input, "PartOf=", "PartOf=stagecore-hub.service", 1) }},
		{"unloaded unit", func(input string) string { return strings.Replace(input, "LoadState=loaded", "LoadState=not-found", 1) }},
		{"shared host network", func(input string) string { return strings.Replace(input, "PrivateNetwork=yes", "PrivateNetwork=no", 1) }},
		{"host devices visible", func(input string) string { return strings.Replace(input, "PrivateDevices=yes", "PrivateDevices=no", 1) }},
		{"open device policy", func(input string) string {
			return strings.Replace(input, "DevicePolicy=closed", "DevicePolicy=auto", 1)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSystemdShowOutput(opts, tc.edit(base)); err == nil {
				t.Fatal("unsafe systemd resolution was accepted")
			}
		})
	}
}
