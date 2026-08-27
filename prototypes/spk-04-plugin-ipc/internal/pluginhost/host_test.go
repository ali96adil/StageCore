package pluginhost_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-04-plugin-ipc/internal/pluginhost"
	"github.com/ali96adil/StageCore/prototypes/spk-04-plugin-ipc/internal/protocol"
)

var pluginBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "stagecore-spk04-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	pluginBinary = filepath.Join(dir, "stagecore-osc-plugin")
	cmd := exec.Command("go", "build", "-o", pluginBinary, "../../cmd/stagecore-osc-plugin")
	cmd.Dir = filepath.Join(".")
	if out, err := cmd.CombinedOutput(); err != nil {
		panic(string(out) + err.Error())
	}
	os.Exit(m.Run())
}

func TestRealOSCThroughExternalPlugin(t *testing.T) {
	listener, port := udpListener(t)
	defer listener.Close()
	h := pluginhost.New(pluginBinary, nil, nil, nil)
	defer h.Close()
	res, err := h.Execute(context.Background(), request("exec-osc-1", port, 500))
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "COMPLETED" || res.AckLevel != "TRANSPORT_ONLY" {
		t.Fatalf("unexpected result: %+v", res)
	}
	buf := make([]byte, 512)
	_ = listener.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := listener.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 || string(buf[:10]) != "/scene/go\x00" {
		t.Fatalf("unexpected OSC packet: %q", buf[:n])
	}
}

func TestCrashDoesNotCrashHostAndRequiresExplicitNewExecution(t *testing.T) {
	listener, port := udpListener(t)
	defer listener.Close()
	marker := filepath.Join(t.TempDir(), "crash.marker")
	h := pluginhost.New(pluginBinary, nil, []string{"STAGECORE_PLUGIN_TEST_FAULT=crash-once", "STAGECORE_PLUGIN_TEST_MARKER=" + marker}, nil)
	defer h.Close()
	res1, err := h.Execute(context.Background(), request("exec-crash-1", port, 500))
	if !errors.Is(err, pluginhost.ErrPluginExited) || res1.Status != "FAILED" {
		t.Fatalf("expected contained crash, got result=%+v err=%v", res1, err)
	}
	// The host does not replay exec-crash-1. A new execution ID is required.
	res2, err := h.Execute(context.Background(), request("exec-after-crash", port, 500))
	if err != nil || res2.Status != "COMPLETED" {
		t.Fatalf("restart failed: result=%+v err=%v", res2, err)
	}
}

func TestHangTimesOutKillsPluginAndNextExplicitExecutionWorks(t *testing.T) {
	listener, port := udpListener(t)
	defer listener.Close()
	marker := filepath.Join(t.TempDir(), "hang.marker")
	h := pluginhost.New(pluginBinary, nil, []string{"STAGECORE_PLUGIN_TEST_FAULT=hang-once", "STAGECORE_PLUGIN_TEST_MARKER=" + marker}, nil)
	defer h.Close()
	res1, err := h.Execute(context.Background(), request("exec-hang-1", port, 120))
	if !errors.Is(err, pluginhost.ErrPluginTimeout) || res1.ErrorCategory != "TIMEOUT" {
		t.Fatalf("expected timeout, got result=%+v err=%v", res1, err)
	}
	res2, err := h.Execute(context.Background(), request("exec-after-hang", port, 500))
	if err != nil || res2.Status != "COMPLETED" {
		t.Fatalf("restart after hang failed: result=%+v err=%v", res2, err)
	}
}

func request(id string, port int, timeout int) protocol.ExecutionRequest {
	params, _ := json.Marshal(map[string]any{"address": "/scene/go", "arguments": []any{float64(4), "intro", true}})
	return protocol.ExecutionRequest{Type: "execution.request", SchemaVersion: protocol.SchemaVersion, ExecutionID: id, Capability: "osc.send", Target: protocol.ResolvedTarget{Host: "127.0.0.1", Port: port}, Parameters: params, Priority: "P1", TimeoutMS: timeout, CorrelationID: "corr-spk04"}
}

func udpListener(t *testing.T) (*net.UDPConn, int) {
	t.Helper()
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	c, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return c, c.LocalAddr().(*net.UDPAddr).Port
}
