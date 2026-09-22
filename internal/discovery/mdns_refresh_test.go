package discovery

import (
	"net"
	"strings"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestJoinedCurrentIPv4ReplacesStaleDHCPAddress(t *testing.T) {
	joined := []discoveryInterface{{
		interfaceInfo: net.Interface{Index: 2, Name: "eth0"},
		addresses:     []net.IP{net.IPv4(192, 168, 3, 131)},
	}}
	current := []discoveryInterface{{
		interfaceInfo: net.Interface{Index: 2, Name: "eth0"},
		addresses:     []net.IP{net.IPv4(192, 168, 3, 130)},
	}}
	selected := joinedCurrentIPv4(joined, current)
	if len(selected) != 1 || len(selected[0].addresses) != 1 ||
		!selected[0].addresses[0].Equal(net.IPv4(192, 168, 3, 130)) {
		t.Fatalf("stale DHCP address was retained: %+v", selected)
	}

	announcement, err := NewAnnouncement(testHubID, "StageCore Hub",
		"SHA256:hub", strings.Repeat("a", 64), "0.0.0.0:7841")
	if err != nil {
		t.Fatal(err)
	}
	packet, err := announcement.BuildPacket(selected[0].addresses, DefaultTTL)
	if err != nil {
		t.Fatal(err)
	}
	var message dnsmessage.Message
	if err := message.Unpack(packet); err != nil {
		t.Fatal(err)
	}
	var foundCurrent bool
	for _, answer := range message.Answers {
		if addr, ok := answer.Body.(*dnsmessage.AResource); ok {
			if addr.A == [4]byte{192, 168, 3, 131} {
				t.Fatal("mDNS packet still advertises the stale IPv4 address")
			}
			if addr.A == [4]byte{192, 168, 3, 130} {
				foundCurrent = true
			}
		}
	}
	if !foundCurrent {
		t.Fatal("mDNS packet did not advertise current IPv4 address")
	}
}

func TestJoinedCurrentIPv4DoesNotUseUnjoinedOrMissingInterfaces(t *testing.T) {
	joined := []discoveryInterface{{
		interfaceInfo: net.Interface{Index: 2, Name: "eth0"},
		addresses:     []net.IP{net.IPv4(192, 168, 3, 131)},
	}}
	for _, tc := range []struct {
		name    string
		current []discoveryInterface
	}{
		{name: "interface removed"},
		{name: "new unjoined interface", current: []discoveryInterface{{
			interfaceInfo: net.Interface{Index: 3, Name: "wlan0"},
			addresses: []net.IP{net.IPv4(192, 168, 3, 130)},
		}}},
		{name: "interface index reused with different name", current: []discoveryInterface{{
			interfaceInfo: net.Interface{Index: 2, Name: "renamed0"},
			addresses: []net.IP{net.IPv4(192, 168, 3, 130)},
		}}},
		{name: "joined interface lost IPv4", current: []discoveryInterface{{
			interfaceInfo: net.Interface{Index: 2, Name: "eth0"},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if selected := joinedCurrentIPv4(joined, tc.current); len(selected) != 0 {
				t.Fatalf("unexpected address on unjoined/unavailable interface: %+v", selected)
			}
		})
	}
}
