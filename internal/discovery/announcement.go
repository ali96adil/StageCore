package discovery

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/net/dns/dnsmessage"
)

const (
	ServiceType      = "_stagecore-hub._tcp"
	serviceFQDN      = ServiceType + ".local."
	serviceEnumFQDN  = "_services._dns-sd._udp.local."
	Domain           = "local."
	DiscoveryVersion = "1"
	DefaultTTL        = uint32(120)
)

type Announcement struct {
	HubID          string
	DisplayName    string
	HubFingerprint string
	TLSCertSHA256  string
	ListenHost     string
	Port           uint16
	HostName       string
	InstanceName   string
	TXT            []string

	serviceName dnsmessage.Name
	enumName    dnsmessage.Name
	hostName    dnsmessage.Name
	instance    dnsmessage.Name
}

func NewAnnouncement(hubID, displayName, hubFingerprint, tlsCertSHA256, deviceListen string) (Announcement, error) {
	hubID = strings.ToLower(strings.TrimSpace(hubID))
	if _, err := uuid.Parse(hubID); err != nil {
		return Announcement{}, fmt.Errorf("invalid Hub ID %q: %w", hubID, err)
	}
	displayName = boundedUTF8(strings.TrimSpace(displayName), 96)
	if displayName == "" {
		displayName = "StageCore Hub"
	}
	hubFingerprint = strings.TrimSpace(hubFingerprint)
	if hubFingerprint == "" || len(hubFingerprint) > 200 {
		return Announcement{}, fmt.Errorf("invalid Hub fingerprint")
	}
	tlsCertSHA256 = strings.ToLower(strings.TrimSpace(tlsCertSHA256))
	if len(tlsCertSHA256) != 64 {
		return Announcement{}, fmt.Errorf("device TLS certificate SHA-256 must be 64 hex characters")
	}
	if _, err := hex.DecodeString(tlsCertSHA256); err != nil {
		return Announcement{}, fmt.Errorf("invalid device TLS certificate SHA-256: %w", err)
	}

	listenHost, portText, err := net.SplitHostPort(strings.TrimSpace(deviceListen))
	if err != nil {
		return Announcement{}, fmt.Errorf("invalid device listen address %q: %w", deviceListen, err)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return Announcement{}, fmt.Errorf("invalid device listen port %q", portText)
	}

	shortID := strings.ReplaceAll(hubID, "-", "")
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	hostFQDN := "stagecore-" + shortID + ".local."
	instanceFQDN := "StageCore-" + shortID + "." + serviceFQDN
	serviceName, err := dnsmessage.NewName(serviceFQDN)
	if err != nil {
		return Announcement{}, err
	}
	enumName, err := dnsmessage.NewName(serviceEnumFQDN)
	if err != nil {
		return Announcement{}, err
	}
	hostName, err := dnsmessage.NewName(hostFQDN)
	if err != nil {
		return Announcement{}, err
	}
	instance, err := dnsmessage.NewName(instanceFQDN)
	if err != nil {
		return Announcement{}, err
	}

	hostWithoutDot := strings.TrimSuffix(hostFQDN, ".")
	announcement := Announcement{
		HubID: hubID, DisplayName: displayName, HubFingerprint: hubFingerprint,
		TLSCertSHA256: tlsCertSHA256, ListenHost: strings.TrimSpace(listenHost), Port: uint16(port),
		HostName: hostWithoutDot, InstanceName: strings.TrimSuffix(instanceFQDN, "."),
		serviceName: serviceName, enumName: enumName, hostName: hostName, instance: instance,
	}
	announcement.TXT = []string{
		"v=" + DiscoveryVersion,
		"hub_id=" + announcement.HubID,
		"name=" + announcement.DisplayName,
		"hub_fp=" + announcement.HubFingerprint,
		"tls_sha256=" + announcement.TLSCertSHA256,
		"host=" + announcement.HostName,
		"port=" + strconv.Itoa(int(announcement.Port)),
		"api_path=/",
		"runtime_path=/api/v1/companion/runtime",
	}
	for _, value := range announcement.TXT {
		if len(value) == 0 || len(value) > 255 || !utf8.ValidString(value) {
			return Announcement{}, fmt.Errorf("invalid discovery TXT value")
		}
	}
	return announcement, nil
}

func (a Announcement) BuildPacket(addresses []net.IP, ttl uint32) ([]byte, error) {
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	builder.EnableCompression()
	if err := builder.StartAnswers(); err != nil {
		return nil, err
	}
	if err := builder.PTRResource(resourceHeader(a.enumName, dnsmessage.TypePTR, ttl), dnsmessage.PTRResource{PTR: a.serviceName}); err != nil {
		return nil, err
	}
	if err := builder.PTRResource(resourceHeader(a.serviceName, dnsmessage.TypePTR, ttl), dnsmessage.PTRResource{PTR: a.instance}); err != nil {
		return nil, err
	}
	if err := builder.SRVResource(resourceHeader(a.instance, dnsmessage.TypeSRV, ttl), dnsmessage.SRVResource{Port: a.Port, Target: a.hostName}); err != nil {
		return nil, err
	}
	if err := builder.TXTResource(resourceHeader(a.instance, dnsmessage.TypeTXT, ttl), dnsmessage.TXTResource{TXT: append([]string(nil), a.TXT...)}); err != nil {
		return nil, err
	}
	for _, address := range addresses {
		ipv4 := address.To4()
		if ipv4 == nil {
			continue
		}
		var raw [4]byte
		copy(raw[:], ipv4)
		if err := builder.AResource(resourceHeader(a.hostName, dnsmessage.TypeA, ttl), dnsmessage.AResource{A: raw}); err != nil {
			return nil, err
		}
	}
	return builder.Finish()
}

func (a Announcement) MatchesQuery(packet []byte) bool {
	var parser dnsmessage.Parser
	if _, err := parser.Start(packet); err != nil {
		return false
	}
	questions, err := parser.AllQuestions()
	if err != nil {
		return false
	}
	for _, question := range questions {
		if question.Name == a.serviceName || question.Name == a.instance || question.Name == a.hostName || question.Name == a.enumName {
			return true
		}
	}
	return false
}

func resourceHeader(name dnsmessage.Name, kind dnsmessage.Type, ttl uint32) dnsmessage.ResourceHeader {
	return dnsmessage.ResourceHeader{Name: name, Type: kind, Class: dnsmessage.ClassINET, TTL: ttl}
}

func boundedUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for len(value) > maxBytes {
		_, size := utf8.DecodeLastRuneInString(value)
		if size <= 0 {
			return ""
		}
		value = value[:len(value)-size]
	}
	return value
}
