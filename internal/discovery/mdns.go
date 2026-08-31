package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/ipv4"
)

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

type Advertiser struct {
	announcement Announcement
	addresses    []net.IP
	interfaces   []net.Interface
	packet       net.PacketConn
	ipv4         *ipv4.PacketConn
	cancel       context.CancelFunc
	closeOnce    sync.Once
}

func Start(ctx context.Context, announcement Announcement) (*Advertiser, error) {
	interfaces, addresses, err := eligibleIPv4Interfaces(announcement.ListenHost)
	if err != nil {
		return nil, err
	}
	if len(interfaces) == 0 || len(addresses) == 0 {
		return nil, fmt.Errorf("no eligible local IPv4 multicast interface for StageCore discovery")
	}

	listenConfig := net.ListenConfig{Control: reuseAddressControl}
	packet, err := listenConfig.ListenPacket(ctx, "udp4", ":5353")
	if err != nil {
		return nil, fmt.Errorf("listen for mDNS: %w", err)
	}
	connection := ipv4.NewPacketConn(packet)
	joined := make([]net.Interface, 0, len(interfaces))
	for index := range interfaces {
		iface := interfaces[index]
		if err := connection.JoinGroup(&iface, mdnsGroup); err == nil {
			joined = append(joined, iface)
		}
	}
	if len(joined) == 0 {
		_ = packet.Close()
		return nil, fmt.Errorf("join mDNS multicast group on local Stage Network interfaces")
	}
	if err := connection.SetMulticastTTL(255); err != nil {
		_ = packet.Close()
		return nil, fmt.Errorf("set mDNS multicast TTL: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	advertiser := &Advertiser{
		announcement: announcement,
		addresses:    addresses,
		interfaces:   joined,
		packet:       packet,
		ipv4:         connection,
		cancel:       cancel,
	}
	go advertiser.run(runCtx)
	return advertiser, nil
}

func (a *Advertiser) Close() error {
	if a == nil {
		return nil
	}
	var closeErr error
	a.closeOnce.Do(func() {
		_ = a.announce(0)
		if a.cancel != nil {
			a.cancel()
		}
		if a.packet != nil {
			closeErr = a.packet.Close()
		}
	})
	return closeErr
}

func (a *Advertiser) run(ctx context.Context) {
	_ = a.announce(DefaultTTL)
	initial := time.NewTimer(time.Second)
	periodic := time.NewTicker(30 * time.Second)
	defer initial.Stop()
	defer periodic.Stop()

	queryWake := make(chan struct{}, 1)
	go a.readQueries(ctx, queryWake)
	for {
		select {
		case <-ctx.Done():
			return
		case <-initial.C:
			_ = a.announce(DefaultTTL)
		case <-periodic.C:
			_ = a.announce(DefaultTTL)
		case <-queryWake:
			_ = a.announce(DefaultTTL)
		}
	}
}

func (a *Advertiser) readQueries(ctx context.Context, wake chan<- struct{}) {
	buffer := make([]byte, 2048)
	for {
		_ = a.packet.SetReadDeadline(time.Now().Add(time.Second))
		count, _, err := a.packet.ReadFrom(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				continue
			}
			continue
		}
		if count > 0 && a.announcement.MatchesQuery(buffer[:count]) {
			select {
			case wake <- struct{}{}:
			default:
			}
		}
	}
}

func (a *Advertiser) announce(ttl uint32) error {
	packet, err := a.announcement.BuildPacket(a.addresses, ttl)
	if err != nil {
		return err
	}
	var firstErr error
	for index := range a.interfaces {
		iface := a.interfaces[index]
		if _, err := a.ipv4.WriteTo(packet, &ipv4.ControlMessage{IfIndex: iface.Index}, mdnsGroup); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func eligibleIPv4Interfaces(listenHost string) ([]net.Interface, []net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("list network interfaces for discovery: %w", err)
	}
	var requested net.IP
	if listenHost != "" && listenHost != "0.0.0.0" {
		requested = net.ParseIP(listenHost)
		if requested == nil || requested.To4() == nil {
			return nil, nil, fmt.Errorf("F-004 IPv4 discovery requires a wildcard or IPv4 device listen host, got %q", listenHost)
		}
		if requested.IsLoopback() {
			return nil, nil, fmt.Errorf("device listener %q is loopback-only and cannot be advertised on the Stage LAN", listenHost)
		}
	}

	selectedInterfaces := []net.Interface{}
	selectedAddresses := []net.IP{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		matched := false
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err != nil || ip.To4() == nil || ip.IsLoopback() {
				continue
			}
			if requested != nil && !ip.Equal(requested) {
				continue
			}
			selectedAddresses = append(selectedAddresses, append(net.IP(nil), ip.To4()...))
			matched = true
		}
		if matched {
			selectedInterfaces = append(selectedInterfaces, iface)
		}
	}
	return selectedInterfaces, selectedAddresses, nil
}

func reuseAddressControl(_ string, _ string, raw syscall.RawConn) error {
	var controlErr error
	if err := raw.Control(func(fd uintptr) {
		controlErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	}); err != nil {
		return err
	}
	return controlErr
}
