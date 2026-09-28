package camerarelay

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

var relaySourceDialer = &net.Dialer{
	Timeout:   3 * time.Second,
	KeepAlive: 20 * time.Second,
}

var mdnsDestination = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

func sourceDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	resolved, err := resolveSourceDialAddress(ctx, address, lookupMDNSIPv4)
	if err != nil {
		return nil, err
	}
	if resolved == address {
		return relaySourceDialer.DialContext(ctx, network, resolved)
	}
	return relaySourceDialer.DialContext(ctx, "tcp4", resolved)
}

func resolveSourceDialAddress(
	ctx context.Context,
	address string,
	lookup func(context.Context, string) (net.IP, error),
) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("invalid camera source address %q: %w", address, err)
	}
	normalized := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if !strings.HasSuffix(normalized, ".local") {
		return address, nil
	}
	if lookup == nil {
		return "", fmt.Errorf("mDNS resolver is unavailable")
	}
	ip, err := lookup(ctx, normalized)
	if err != nil {
		return "", err
	}
	ipv4 := ip.To4()
	if ipv4 == nil || ipv4.IsUnspecified() || ipv4.IsLoopback() || ipv4.IsMulticast() {
		return "", fmt.Errorf("mDNS returned an unusable IPv4 address for %s", normalized)
	}
	return net.JoinHostPort(ipv4.String(), port), nil
}

func lookupMDNSIPv4(ctx context.Context, host string) (net.IP, error) {
	normalized := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if normalized == "" || !strings.HasSuffix(normalized, ".local") {
		return nil, fmt.Errorf("mDNS host must end in .local")
	}
	name, err := dnsmessage.NewName(normalized + ".")
	if err != nil {
		return nil, fmt.Errorf("invalid mDNS host %q: %w", normalized, err)
	}

	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	if err := builder.StartQuestions(); err != nil {
		return nil, err
	}
	// RFC 6762 QU bit asks responders to send the answer directly back to this
	// ephemeral UDP source port. This avoids competing with the Hub advertiser
	// already bound to UDP/5353 on the same Pi.
	if err := builder.Question(dnsmessage.Question{
		Name:  name,
		Type:  dnsmessage.TypeA,
		Class: dnsmessage.Class(uint16(dnsmessage.ClassINET) | 0x8000),
	}); err != nil {
		return nil, err
	}
	query, err := builder.Finish()
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("open mDNS query socket: %w", err)
	}
	defer conn.Close()
	if _, err := conn.WriteToUDP(query, mdnsDestination); err != nil {
		return nil, fmt.Errorf("send mDNS query: %w", err)
	}

	deadline := time.Now().Add(1500 * time.Millisecond)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	buffer := make([]byte, 4096)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stepDeadline := time.Now().Add(250 * time.Millisecond)
		if stepDeadline.After(deadline) {
			stepDeadline = deadline
		}
		if err := conn.SetReadDeadline(stepDeadline); err != nil {
			return nil, err
		}
		count, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				continue
			}
			return nil, fmt.Errorf("read mDNS response: %w", err)
		}
		if ip := matchingMDNSIPv4(buffer[:count], normalized); ip != nil {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("mDNS lookup for %s returned no IPv4 answer", normalized)
}

func matchingMDNSIPv4(packet []byte, host string) net.IP {
	var parser dnsmessage.Parser
	if _, err := parser.Start(packet); err != nil {
		return nil
	}
	if _, err := parser.AllQuestions(); err != nil {
		return nil
	}
	answers, err := parser.AllAnswers()
	if err != nil {
		return nil
	}
	if _, err := parser.AllAuthorities(); err != nil {
		return nil
	}
	additionals, err := parser.AllAdditionals()
	if err != nil {
		return nil
	}
	for _, resource := range append(answers, additionals...) {
		if !sameMDNSName(resource.Header.Name.String(), host) {
			continue
		}
		a, ok := resource.Body.(*dnsmessage.AResource)
		if !ok {
			continue
		}
		ip := net.IPv4(a.A[0], a.A[1], a.A[2], a.A[3])
		if ipv4 := ip.To4(); ipv4 != nil {
			return append(net.IP(nil), ipv4...)
		}
	}
	return nil
}

func sameMDNSName(answer, host string) bool {
	return strings.EqualFold(
		strings.TrimSuffix(strings.TrimSpace(answer), "."),
		strings.TrimSuffix(strings.TrimSpace(host), "."),
	)
}
