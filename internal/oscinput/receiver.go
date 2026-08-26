package oscinput

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/routing"
)

type Receiver struct {
	conn      *net.UDPConn
	engine    *routing.Engine
	sessionID string
}

func Listen(address string, engine *routing.Engine, sessionID string) (*Receiver, error) {
	if engine == nil {
		return nil, fmt.Errorf("routing engine is required")
	}
	udpAddress, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("resolve OSC input listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp", udpAddress)
	if err != nil {
		return nil, fmt.Errorf("listen OSC input: %w", err)
	}
	return &Receiver{conn: conn, engine: engine, sessionID: sessionID}, nil
}

func (r *Receiver) LocalAddr() *net.UDPAddr {
	if r == nil || r.conn == nil {
		return nil
	}
	addr, _ := r.conn.LocalAddr().(*net.UDPAddr)
	return addr
}

func (r *Receiver) Serve(ctx context.Context) error {
	if r == nil || r.conn == nil || r.engine == nil {
		return fmt.Errorf("OSC input receiver is not initialized")
	}
	buffer := make([]byte, 64*1024)
	for {
		if err := r.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
			return err
		}
		n, _, err := r.conn.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var networkError net.Error
			if errors.As(err, &networkError) && networkError.Timeout() {
				continue
			}
			return fmt.Errorf("read OSC input: %w", err)
		}
		message, err := osc.DecodeMessage(buffer[:n])
		if err != nil {
			// Malformed UDP input is isolated to this datagram; the receiver stays
			// available for later valid show traffic.
			continue
		}
		arguments := make([]any, 0, len(message.Arguments))
		for _, argument := range message.Arguments {
			arguments = append(arguments, argument.Value)
		}
		_, _ = r.engine.ReceiveOSC(ctx, r.sessionID, message.Address, arguments)
	}
}

func (r *Receiver) Close() error {
	if r == nil || r.conn == nil {
		return nil
	}
	return r.conn.Close()
}
