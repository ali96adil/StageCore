package extension

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRuntimeNetworkBrokerSendsApprovedUDPAndCleansSession(t *testing.T) {
	h := newDependencyTestHarness(t)
	broker, err := NewRuntimeNetworkBroker(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	session, err := broker.OpenSession([]string{RuntimeNetworkBrokerPermissionUDPSend})
	if err != nil {
		t.Fatal(err)
	}

	udpListener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer udpListener.Close()
	target := udpListener.LocalAddr().(*net.UDPAddr)

	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: session.HostSocketPath(), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("stagecore broker udp qualification")
	request := RuntimeNetworkBrokerRequest{
		Type:          RuntimeNetworkBrokerRequestType,
		SchemaVersion: RuntimeNetworkBrokerSchemaVersion,
		RequestID:     "req-1",
		Operation:     RuntimeNetworkBrokerOperationUDPSend,
		Token:         session.Token(),
		Host:          "127.0.0.1",
		Port:          target.Port,
		PayloadBase64: base64.StdEncoding.EncodeToString(payload),
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		t.Fatal(err)
	}
	var response RuntimeNetworkBrokerResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if response.Status != RuntimeNetworkBrokerStatusCompleted || response.BytesSent != len(payload) || response.ErrorCode != "" {
		t.Fatalf("broker response=%+v", response)
	}

	buffer := make([]byte, 256)
	if err := udpListener.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	n, _, err := udpListener.ReadFromUDP(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if string(buffer[:n]) != string(payload) {
		t.Fatalf("udp payload=%q want=%q", buffer[:n], payload)
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(broker.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("broker session leaked after close: %v", entries)
	}
}

func TestRuntimeNetworkBrokerRejectsHostnameBadTokenAndUnapprovedOperation(t *testing.T) {
	h := newDependencyTestHarness(t)
	broker, err := NewRuntimeNetworkBroker(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	session, err := broker.OpenSession(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	call := func(request RuntimeNetworkBrokerRequest) RuntimeNetworkBrokerResponse {
		t.Helper()
		conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: session.HostSocketPath(), Net: "unix"})
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if err := json.NewEncoder(conn).Encode(request); err != nil {
			t.Fatal(err)
		}
		var response RuntimeNetworkBrokerResponse
		if err := json.NewDecoder(conn).Decode(&response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	base := RuntimeNetworkBrokerRequest{
		Type:          RuntimeNetworkBrokerRequestType,
		SchemaVersion: RuntimeNetworkBrokerSchemaVersion,
		RequestID:     "req",
		Operation:     RuntimeNetworkBrokerOperationUDPSend,
		Token:         session.Token(),
		Host:          "127.0.0.1",
		Port:          9000,
		PayloadBase64: base64.StdEncoding.EncodeToString([]byte("x")),
	}
	badToken := base
	badToken.Token = strings.Repeat("0", len(session.Token()))
	if got := call(badToken); got.ErrorCode != RuntimeNetworkBrokerErrorAuth {
		t.Fatalf("bad token response=%+v", got)
	}

	unapproved := base
	if got := call(unapproved); got.ErrorCode != RuntimeNetworkBrokerErrorPermission {
		t.Fatalf("unapproved response=%+v", got)
	}

	approved, err := broker.OpenSession([]string{RuntimeNetworkBrokerPermissionUDPSend})
	if err != nil {
		t.Fatal(err)
	}
	defer approved.Close()
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: approved.HostSocketPath(), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	hostname := base
	hostname.Token = approved.Token()
	hostname.Host = "localhost"
	if err := json.NewEncoder(conn).Encode(hostname); err != nil {
		t.Fatal(err)
	}
	var response RuntimeNetworkBrokerResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.ErrorCode != RuntimeNetworkBrokerErrorTarget {
		t.Fatalf("hostname response=%+v", response)
	}
}

func TestRuntimeNetworkBrokerSupportsOnlyUDPSend(t *testing.T) {
	h := newDependencyTestHarness(t)
	broker, err := NewRuntimeNetworkBroker(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	if !broker.Supports([]string{"non.network.permission", RuntimeNetworkBrokerPermissionUDPSend}) {
		t.Fatal("network.udp.send should be broker-supported")
	}
	if broker.Supports([]string{"network.udp.listen"}) {
		t.Fatal("network.udp.listen must remain fail-closed")
	}
}
