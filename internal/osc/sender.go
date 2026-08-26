package osc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type Endpoint struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type SendResult struct {
	Status    string
	AckLevel  string
	BytesSent int
	Duration  time.Duration
}

type Sender struct {
	WriteTimeout time.Duration
}

func (s Sender) Send(ctx context.Context, endpoint Endpoint, msg Message) (SendResult, error) {
	if strings.TrimSpace(endpoint.Host) == "" {
		return SendResult{Status: "FAILED", AckLevel: "NONE"}, errors.New("OSC endpoint host is required")
	}
	if endpoint.Port < 1 || endpoint.Port > 65535 {
		return SendResult{Status: "FAILED", AckLevel: "NONE"}, errors.New("OSC endpoint port must be 1..65535")
	}
	packet, err := EncodeMessage(msg)
	if err != nil {
		return SendResult{Status: "FAILED", AckLevel: "NONE"}, err
	}

	started := time.Now()
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port)))
	if err != nil {
		return SendResult{Status: "FAILED", AckLevel: "NONE", Duration: time.Since(started)}, fmt.Errorf("resolve OSC endpoint: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return SendResult{Status: "FAILED", AckLevel: "NONE", Duration: time.Since(started)}, fmt.Errorf("open OSC UDP socket: %w", err)
	}
	defer conn.Close()

	timeout := s.WriteTimeout
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return SendResult{Status: "FAILED", AckLevel: "NONE", Duration: time.Since(started)}, fmt.Errorf("set OSC write deadline: %w", err)
	}

	select {
	case <-ctx.Done():
		return SendResult{Status: "CANCELLED", AckLevel: "NONE", Duration: time.Since(started)}, ctx.Err()
	default:
	}

	n, err := conn.Write(packet)
	if err != nil {
		status := "FAILED"
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			status = "TIMED_OUT"
		}
		return SendResult{Status: status, AckLevel: "NONE", BytesSent: n, Duration: time.Since(started)}, fmt.Errorf("send OSC UDP packet: %w", err)
	}
	if n != len(packet) {
		return SendResult{Status: "FAILED", AckLevel: "NONE", BytesSent: n, Duration: time.Since(started)}, fmt.Errorf("short OSC UDP write: %d of %d bytes", n, len(packet))
	}
	return SendResult{Status: "COMPLETED", AckLevel: "TRANSPORT_ONLY", BytesSent: n, Duration: time.Since(started)}, nil
}
