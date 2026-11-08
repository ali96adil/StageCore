package companionchannel

import "testing"

func TestValidOSCQueryDiscoveryResult(t *testing.T) {
	min, max := 0.0, 1.0
	good := OSCQueryDiscoveryResult{
		RequestID: "test", Status: "COMPLETED", Name: "VDMX6-OSCQuery", OSCPort: 3546,
		Controls: []OSCQueryControl{{
			Path: "/OSCQUERY/Control Surface/Slider", Type: "f", Minimum: &min, Maximum: &max,
		}},
	}
	if !validOSCQueryDiscoveryResult(good) {
		t.Fatal("valid VDMX slider rejected")
	}
	for _, modify := range []func(*OSCQueryDiscoveryResult){
		func(r *OSCQueryDiscoveryResult) { r.OSCPort = 0 },
		func(r *OSCQueryDiscoveryResult) { r.Name = "" },
		func(r *OSCQueryDiscoveryResult) { r.Controls[0].Path = "bad" },
		func(r *OSCQueryDiscoveryResult) { r.Controls[0].Minimum, r.Controls[0].Maximum = &max, &min },
	} {
		v := good
		v.Controls = append([]OSCQueryControl(nil), good.Controls...)
		modify(&v)
		if validOSCQueryDiscoveryResult(v) {
			t.Fatalf("invalid payload accepted: %#v", v)
		}
	}
}
