package camerarelay

import (
	"context"
	"net"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestResolveSourceDialAddressUsesMDNSForLocalHost(t *testing.T) {
	called := false
	got, err := resolveSourceDialAddress(
		context.Background(),
		"stagecam-d44a4c.local:81",
		func(_ context.Context, host string) (net.IP, error) {
			called = true
			if host != "stagecam-d44a4c.local" {
				t.Fatalf("unexpected lookup host %q", host)
			}
			return net.IPv4(192, 168, 3, 143), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected mDNS lookup")
	}
	if got != "192.168.3.143:81" {
		t.Fatalf("resolved address = %q", got)
	}
}

func TestResolveSourceDialAddressLeavesOrdinaryHostAlone(t *testing.T) {
	got, err := resolveSourceDialAddress(
		context.Background(),
		"192.168.3.143:81",
		func(context.Context, string) (net.IP, error) {
			t.Fatal("ordinary address must not use mDNS lookup")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "192.168.3.143:81" {
		t.Fatalf("resolved address = %q", got)
	}
}

func TestMatchingMDNSIPv4FindsMatchingAdditionalRecord(t *testing.T) {
	host, err := dnsmessage.NewName("stagecam-d44a4c.local.")
	if err != nil {
		t.Fatal(err)
	}
	other, err := dnsmessage.NewName("other.local.")
	if err != nil {
		t.Fatal(err)
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	if err := builder.StartAnswers(); err != nil {
		t.Fatal(err)
	}
	if err := builder.AResource(
		dnsmessage.ResourceHeader{Name: other, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 120},
		dnsmessage.AResource{A: [4]byte{10, 0, 0, 1}},
	); err != nil {
		t.Fatal(err)
	}
	if err := builder.StartAdditionals(); err != nil {
		t.Fatal(err)
	}
	if err := builder.AResource(
		dnsmessage.ResourceHeader{Name: host, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 120},
		dnsmessage.AResource{A: [4]byte{192, 168, 3, 143}},
	); err != nil {
		t.Fatal(err)
	}
	packet, err := builder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	got := matchingMDNSIPv4(packet, "stagecam-d44a4c.local")
	if got == nil || !got.Equal(net.IPv4(192, 168, 3, 143)) {
		t.Fatalf("matching IP = %v", got)
	}
}

func TestMatchingMDNSIPv4IgnoresDifferentHost(t *testing.T) {
	host, err := dnsmessage.NewName("other.local.")
	if err != nil {
		t.Fatal(err)
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	if err := builder.StartAnswers(); err != nil {
		t.Fatal(err)
	}
	if err := builder.AResource(
		dnsmessage.ResourceHeader{Name: host, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 120},
		dnsmessage.AResource{A: [4]byte{192, 168, 3, 143}},
	); err != nil {
		t.Fatal(err)
	}
	packet, err := builder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if got := matchingMDNSIPv4(packet, "stagecam-d44a4c.local"); got != nil {
		t.Fatalf("unexpected matching IP %v", got)
	}
}
