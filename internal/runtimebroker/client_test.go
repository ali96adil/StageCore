package runtimebroker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testUnixListener(t *testing.T) (*net.UnixListener, string) {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "sc-rbc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	path := filepath.Join(directory, "b.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener, path
}

func TestClientSendUDPUsesBrokerContract(t *testing.T) {
	listener, path := testUnixListener(t)
	requestCh := make(chan Request, 1)
	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		var request Request
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			return
		}
		requestCh <- request
		payload, _ := base64.StdEncoding.DecodeString(request.PayloadBase64)
		_ = json.NewEncoder(conn).Encode(Response{
			Type:          ResultType,
			SchemaVersion: SchemaVersion,
			RequestID:     request.RequestID,
			Status:        StatusCompleted,
			BytesSent:     len(payload),
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	payload := []byte("broker client payload")
	bytesSent, err := (Client{SocketPath: path, Token: "test-token"}).SendUDP(ctx, "127.0.0.1", 9000, payload)
	if err != nil {
		t.Fatal(err)
	}
	if bytesSent != len(payload) {
		t.Fatalf("bytes sent=%d want=%d", bytesSent, len(payload))
	}
	request := <-requestCh
	if request.Type != RequestType || request.SchemaVersion != SchemaVersion || request.Operation != OperationUDPSend || request.Token != "test-token" || request.Host != "127.0.0.1" || request.Port != 9000 || request.RequestID == "" {
		t.Fatalf("request=%+v", request)
	}
	decoded, err := base64.StdEncoding.DecodeString(request.PayloadBase64)
	if err != nil || string(decoded) != string(payload) {
		t.Fatalf("payload=%q err=%v", decoded, err)
	}
}

func TestClientReturnsBoundedBrokerError(t *testing.T) {
	listener, path := testUnixListener(t)
	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		var request Request
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{
			Type:          ResultType,
			SchemaVersion: SchemaVersion,
			RequestID:     request.RequestID,
			Status:        StatusFailed,
			ErrorCode:     ErrorTarget,
			ErrorMessage:  "target rejected",
		})
	}()

	_, err := (Client{SocketPath: path, Token: "test-token"}).SendUDP(context.Background(), "localhost", 9000, []byte("x"))
	var brokerErr *BrokerError
	if !errors.As(err, &brokerErr) || brokerErr.Code != ErrorTarget {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestFromEnvironmentFailsClosedOnPartialContract(t *testing.T) {
	oldSocket, hadSocket := os.LookupEnv(SocketEnv)
	oldToken, hadToken := os.LookupEnv(TokenEnv)
	_ = os.Unsetenv(SocketEnv)
	_ = os.Unsetenv(TokenEnv)
	t.Cleanup(func() {
		if hadSocket { _ = os.Setenv(SocketEnv, oldSocket) } else { _ = os.Unsetenv(SocketEnv) }
		if hadToken { _ = os.Setenv(TokenEnv, oldToken) } else { _ = os.Unsetenv(TokenEnv) }
	})

	if _, configured, err := FromEnvironment(); err != nil || configured {
		t.Fatalf("absent contract configured=%v err=%v", configured, err)
	}
	if err := os.Setenv(SocketEnv, "/stagecore/network/n.sock"); err != nil {
		t.Fatal(err)
	}
	if _, configured, err := FromEnvironment(); !configured || !errors.Is(err, ErrConfiguration) {
		t.Fatalf("partial contract configured=%v err=%v", configured, err)
	}
}
