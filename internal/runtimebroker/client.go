package runtimebroker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/id"
)

const defaultClientTimeout = 500 * time.Millisecond

var ErrConfiguration = errors.New("runtime network broker configuration is invalid")

type BrokerError struct {
	Code    string
	Message string
}

func (e *BrokerError) Error() string {
	if e == nil {
		return "runtime network broker request failed"
	}
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Sprintf("runtime network broker request failed: %s", e.Code)
	}
	return fmt.Sprintf("runtime network broker request failed: %s: %s", e.Code, e.Message)
}

type Client struct {
	SocketPath string
	Token      string
}

// FromEnvironment returns configured=false when the broker contract is absent.
// A partial contract fails closed instead of allowing a caller to fall back to
// direct networking.
func FromEnvironment() (client Client, configured bool, err error) {
	socket, socketSet := os.LookupEnv(SocketEnv)
	token, tokenSet := os.LookupEnv(TokenEnv)
	socket = strings.TrimSpace(socket)
	token = strings.TrimSpace(token)
	if !socketSet && !tokenSet {
		return Client{}, false, nil
	}
	if !socketSet || !tokenSet || socket == "" || token == "" {
		return Client{}, true, fmt.Errorf("%w: broker socket and token must both be present", ErrConfiguration)
	}
	return Client{SocketPath: socket, Token: token}, true, nil
}

func (c Client) SendUDP(ctx context.Context, host string, port int, payload []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	socketPath := filepath.Clean(strings.TrimSpace(c.SocketPath))
	if !filepath.IsAbs(socketPath) || strings.TrimSpace(c.Token) == "" {
		return 0, fmt.Errorf("%w: broker socket must be absolute and token non-empty", ErrConfiguration)
	}
	requestID, err := id.New()
	if err != nil {
		return 0, err
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return 0, fmt.Errorf("connect runtime network broker: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(defaultClientTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return 0, fmt.Errorf("set runtime network broker deadline: %w", err)
	}

	request := Request{
		Type:          RequestType,
		SchemaVersion: SchemaVersion,
		RequestID:     requestID,
		Operation:     OperationUDPSend,
		Token:         c.Token,
		Host:          strings.TrimSpace(host),
		Port:          port,
		PayloadBase64: base64.StdEncoding.EncodeToString(payload),
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return 0, fmt.Errorf("write runtime network broker request: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	var response Response
	decoder := json.NewDecoder(conn)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return 0, fmt.Errorf("read runtime network broker response: %w", err)
	}
	if response.Type != ResultType || response.SchemaVersion != SchemaVersion || response.RequestID != requestID {
		return 0, fmt.Errorf("runtime network broker response contract mismatch")
	}
	if response.Status != StatusCompleted {
		return 0, &BrokerError{Code: response.ErrorCode, Message: response.ErrorMessage}
	}
	if response.ErrorCode != "" || response.BytesSent != len(payload) {
		return 0, fmt.Errorf("runtime network broker success response contract mismatch")
	}
	return response.BytesSent, nil
}
