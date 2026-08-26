package pluginhost_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
)

func TestOSCPluginCrashAndHangAreContainedWithoutReplay(t *testing.T) {
	pluginPath := buildOSCPlugin(t)

	for _, tc := range []struct {
		name      string
		fault     string
		timeoutMS int64
		wantErr   error
	}{
		{name: "crash", fault: "crash-once", timeoutMS: 300, wantErr: pluginhost.ErrPluginExited},
		{name: "hang", fault: "hang-once", timeoutMS: 80, wantErr: pluginhost.ErrPluginTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			port := conn.LocalAddr().(*net.UDPAddr).Port

			marker := filepath.Join(t.TempDir(), "fault.marker")
			host := pluginhost.New(
				pluginPath,
				nil,
				[]string{
					"STAGECORE_PLUGIN_TEST_FAULT=" + tc.fault,
					"STAGECORE_PLUGIN_TEST_MARKER=" + marker,
				},
				nil,
				oscManifest([]string{"network.udp.send"}),
			)
			defer host.Close()

			first := request("exec-1", port, tc.timeoutMS)
			_, err = host.Execute(context.Background(), first)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("first err=%v want %v", err, tc.wantErr)
			}

			second := request("exec-2", port, 300)
			result, err := host.Execute(context.Background(), second)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "COMPLETED" || result.AckLevel != "TRANSPORT_ONLY" {
				t.Fatalf("second result=%#v", result)
			}

			packet := make([]byte, 2048)
			if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			n, _, err := conn.ReadFromUDP(packet)
			if err != nil {
				t.Fatal(err)
			}
			msg, err := osc.DecodeMessage(packet[:n])
			if err != nil {
				t.Fatal(err)
			}
			if msg.Address != "/plugin/test" {
				t.Fatalf("message=%#v", msg)
			}

			_ = conn.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
			if _, _, err := conn.ReadFromUDP(packet); err == nil {
				t.Fatal("unexpected second datagram: failed execution was replayed")
			}
		})
	}
}

func TestPermissionDeniedBeforePluginStart(t *testing.T) {
	host := pluginhost.New(
		"/definitely/not/a/plugin",
		nil,
		nil,
		nil,
		oscManifest(nil),
	)
	defer host.Close()

	result, err := host.Execute(context.Background(), request("denied", 53000, 100))
	if !errors.Is(err, pluginhost.ErrPluginPermissionDenied) {
		t.Fatalf("err=%v", err)
	}
	if result.ErrorCode != "PLUGIN_PERMISSION_DENIED" {
		t.Fatalf("result=%#v", result)
	}
	if host.Ready() != nil {
		t.Fatal("permission denial should occur before plugin process start")
	}
}

func oscManifest(grants []string) pluginhost.Manifest {
	return pluginhost.Manifest{
		PluginID: "stagecore.osc",
		CapabilityPermissions: map[string][]string{
			"osc.send": {"network.udp.send"},
		},
		GrantedPermissions: grants,
	}
}

func request(id string, port int, timeoutMS int64) pluginprotocol.ExecutionRequest {
	params, _ := json.Marshal(map[string]any{
		"address": "/plugin/test",
		"arguments": []map[string]any{
			{"type": "int32", "value": 7},
		},
	})
	return pluginprotocol.ExecutionRequest{
		Type:          "execution.request",
		SchemaVersion: pluginprotocol.SchemaVersion,
		ExecutionID:   id,
		Capability:    "osc.send",
		Target:        pluginprotocol.ResolvedTarget{Host: "127.0.0.1", Port: port},
		Parameters:    params,
		Priority:      "P1",
		TimeoutMS:     timeoutMS,
		CorrelationID: "corr-test",
	}
}

func buildOSCPlugin(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	binary := filepath.Join(t.TempDir(), "stagecore-osc-plugin")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/stagecore-osc-plugin")
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build OSC plugin: %v\n%s", err, output)
	}
	return binary
}
