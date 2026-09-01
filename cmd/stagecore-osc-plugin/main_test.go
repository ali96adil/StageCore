package main

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

	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
	"github.com/ali96adil/StageCore/internal/runtimebroker"
)

func clearBrokerEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{runtimebroker.SocketEnv, runtimebroker.TokenEnv} {
		old, had := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		key, old, had := key, old, had
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}

func fakeBroker(t *testing.T, responder func(runtimebroker.Request) runtimebroker.Response) (string, <-chan runtimebroker.Request) {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "sc-osc-broker-")
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
	requests := make(chan runtimebroker.Request, 1)
	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		var request runtimebroker.Request
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			return
		}
		requests <- request
		response := responder(request)
		_ = json.NewEncoder(conn).Encode(response)
	}()
	return path, requests
}

func testOSCMessage(t *testing.T) (osc.Message, []byte) {
	t.Helper()
	message := osc.Message{Address: "/stagecore/broker/test", Arguments: []osc.Argument{{Type: "int32", Value: 7}}}
	packet, err := osc.EncodeMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	return message, packet
}

func TestSendOSCUsesRuntimeBrokerWhenConfigured(t *testing.T) {
	clearBrokerEnvironment(t)
	path, requests := fakeBroker(t, func(request runtimebroker.Request) runtimebroker.Response {
		payload, _ := base64.StdEncoding.DecodeString(request.PayloadBase64)
		return runtimebroker.Response{
			Type:          runtimebroker.ResultType,
			SchemaVersion: runtimebroker.SchemaVersion,
			RequestID:     request.RequestID,
			Status:        runtimebroker.StatusCompleted,
			BytesSent:     len(payload),
		}
	})
	if err := os.Setenv(runtimebroker.SocketEnv, path); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv(runtimebroker.TokenEnv, "broker-token"); err != nil {
		t.Fatal(err)
	}
	message, packet := testOSCMessage(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := sendOSC(ctx, osc.Endpoint{Host: "127.0.0.1", Port: 9000}, message, packet, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "COMPLETED" || result.AckLevel != "TRANSPORT_ONLY" || result.BytesSent != len(packet) {
		t.Fatalf("send result=%+v", result)
	}
	request := <-requests
	if request.Operation != runtimebroker.OperationUDPSend || request.Token != "broker-token" || request.Host != "127.0.0.1" || request.Port != 9000 {
		t.Fatalf("broker request=%+v", request)
	}
	payload, err := base64.StdEncoding.DecodeString(request.PayloadBase64)
	if err != nil || string(payload) != string(packet) {
		t.Fatalf("broker payload=%q err=%v", payload, err)
	}
}

func TestSendOSCFailsClosedOnPartialBrokerEnvironment(t *testing.T) {
	clearBrokerEnvironment(t)
	if err := os.Setenv(runtimebroker.SocketEnv, "/stagecore/network/n.sock"); err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	message, packet := testOSCMessage(t)
	target := listener.LocalAddr().(*net.UDPAddr)

	result, err := sendOSC(context.Background(), osc.Endpoint{Host: "127.0.0.1", Port: target.Port}, message, packet, time.Second)
	if !errors.Is(err, runtimebroker.ErrConfiguration) || result.Status != "FAILED" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	_ = listener.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	buffer := make([]byte, 256)
	if _, _, readErr := listener.ReadFromUDP(buffer); readErr == nil {
		t.Fatal("partial broker configuration unexpectedly fell back to direct UDP")
	}
}

func TestSendOSCPreservesDirectUDPPathWhenBrokerAbsent(t *testing.T) {
	clearBrokerEnvironment(t)
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	message, packet := testOSCMessage(t)
	target := listener.LocalAddr().(*net.UDPAddr)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := sendOSC(ctx, osc.Endpoint{Host: "127.0.0.1", Port: target.Port}, message, packet, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "COMPLETED" || result.AckLevel != "TRANSPORT_ONLY" {
		t.Fatalf("direct result=%+v", result)
	}
	_ = listener.SetReadDeadline(time.Now().Add(time.Second))
	buffer := make([]byte, 256)
	n, _, err := listener.ReadFromUDP(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if string(buffer[:n]) != string(packet) {
		t.Fatalf("direct packet=%q want=%q", buffer[:n], packet)
	}
}

func TestExecuteMapsBrokerRejectionToNetworkFailure(t *testing.T) {
	clearBrokerEnvironment(t)
	path, _ := fakeBroker(t, func(request runtimebroker.Request) runtimebroker.Response {
		return runtimebroker.Response{
			Type:          runtimebroker.ResultType,
			SchemaVersion: runtimebroker.SchemaVersion,
			RequestID:     request.RequestID,
			Status:        runtimebroker.StatusFailed,
			ErrorCode:     runtimebroker.ErrorTarget,
			ErrorMessage:  "target rejected",
		}
	})
	_ = os.Setenv(runtimebroker.SocketEnv, path)
	_ = os.Setenv(runtimebroker.TokenEnv, "broker-token")
	parameters, err := json.Marshal(oscParams{Address: "/stagecore/test"})
	if err != nil {
		t.Fatal(err)
	}
	result := execute(pluginprotocol.ExecutionRequest{
		ExecutionID: "exec-broker-reject",
		Capability:  capabilityOSCSend,
		Target:      pluginprotocol.ResolvedTarget{Host: "localhost", Port: 9000},
		Parameters:  parameters,
		TimeoutMS:   500,
	})
	if result.Status != "FAILED" || result.ErrorCode != "OSC_SEND_FAILED" || result.ErrorCategory != "NETWORK" {
		t.Fatalf("execution result=%+v", result)
	}
}
