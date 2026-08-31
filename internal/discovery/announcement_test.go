package discovery

import (
	"net"
	"strings"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

const testHubID = "01a045ef-1d7d-7b9b-8bb3-c0daa63fc19d"

func TestAnnouncementCarriesStablePublicIdentity(t *testing.T) {
	announcement, err := NewAnnouncement(
		testHubID,
		"Main Stage Hub",
		"SHA256:example-public-identity",
		strings.Repeat("a", 64),
		"0.0.0.0:7841",
	)
	if err != nil {
		t.Fatal(err)
	}
	if announcement.Port != 7841 || !strings.HasSuffix(announcement.HostName, ".local") {
		t.Fatalf("unexpected endpoint metadata: %+v", announcement)
	}
	joined := strings.Join(announcement.TXT, "\n")
	for _, marker := range []string{
		"v=1",
		"hub_id=" + testHubID,
		"name=Main Stage Hub",
		"hub_fp=SHA256:example-public-identity",
		"tls_sha256=" + strings.Repeat("a", 64),
		"port=7841",
		"runtime_path=/api/v1/companion/runtime",
	} {
		if !strings.Contains(joined, marker) {
			t.Fatalf("TXT contract missing %q: %v", marker, announcement.TXT)
		}
	}
	for _, forbidden := range []string{"password", "token", "pairing_code", "private_key", "project"} {
		if strings.Contains(strings.ToLower(joined), forbidden) {
			t.Fatalf("discovery TXT leaked forbidden marker %q", forbidden)
		}
	}
}

func TestAnnouncementPacketContainsPTRSRVTXTAndAddress(t *testing.T) {
	announcement, err := NewAnnouncement(
		testHubID,
		"StageCore Hub",
		"SHA256:hub",
		strings.Repeat("b", 64),
		"0.0.0.0:7841",
	)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := announcement.BuildPacket([]net.IP{net.IPv4(10, 20, 30, 40)}, DefaultTTL)
	if err != nil {
		t.Fatal(err)
	}
	var message dnsmessage.Message
	if err := message.Unpack(packet); err != nil {
		t.Fatalf("unpack mDNS announcement: %v", err)
	}
	var ptr, srv, txt, address bool
	for _, answer := range message.Answers {
		switch body := answer.Body.(type) {
		case *dnsmessage.PTRResource:
			ptr = ptr || strings.Contains(body.PTR.String(), ServiceType)
		case *dnsmessage.SRVResource:
			srv = srv || body.Port == 7841
		case *dnsmessage.TXTResource:
			txt = txt || strings.Contains(strings.Join(body.TXT, "\n"), "hub_id="+testHubID)
		case *dnsmessage.AResource:
			address = address || body.A == [4]byte{10, 20, 30, 40}
		}
	}
	if !ptr || !srv || !txt || !address {
		t.Fatalf("announcement resources ptr=%v srv=%v txt=%v address=%v answers=%#v", ptr, srv, txt, address, message.Answers)
	}
}

func TestAnnouncementMatchesBonjourBrowseQuestion(t *testing.T) {
	announcement, err := NewAnnouncement(testHubID, "Hub", "SHA256:hub", strings.Repeat("c", 64), "0.0.0.0:7841")
	if err != nil {
		t.Fatal(err)
	}
	name, err := dnsmessage.NewName(serviceFQDN)
	if err != nil {
		t.Fatal(err)
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	if err := builder.StartQuestions(); err != nil {
		t.Fatal(err)
	}
	if err := builder.Question(dnsmessage.Question{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}); err != nil {
		t.Fatal(err)
	}
	query, err := builder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if !announcement.MatchesQuery(query) {
		t.Fatal("StageCore discovery announcement did not match Bonjour service browse query")
	}
}

func TestAnnouncementRejectsMalformedCertificatePin(t *testing.T) {
	if _, err := NewAnnouncement(testHubID, "Hub", "SHA256:hub", "not-a-pin", "0.0.0.0:7841"); err == nil {
		t.Fatal("malformed TLS pin unexpectedly accepted")
	}
}
