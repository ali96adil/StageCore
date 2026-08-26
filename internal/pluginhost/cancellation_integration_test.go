package pluginhost_test

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/pluginhost"
)

func TestOSCPluginCancellationIsTruthfulAndDoesNotReplay(t *testing.T) {
	pluginPath := buildFaultOSCPlugin(t)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	port := conn.LocalAddr().(*net.UDPAddr).Port

	marker := filepath.Join(t.TempDir(), "cancel.marker")
	host := pluginhost.New(
		pluginPath,
		nil,
		[]string{
			"STAGECORE_PLUGIN_TEST_FAULT=hang-once",
			"STAGECORE_PLUGIN_TEST_MARKER=" + marker,
		},
		nil,
		oscManifest([]string{"network.udp.send"}),
	)
	defer host.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	result, err := host.Execute(ctx, request("exec-cancelled", port, 500))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
	if result.Status != "CANCELLED" || result.ErrorCode != "CANCELLED" {
		t.Fatalf("cancelled result=%#v", result)
	}

	next, err := host.Execute(context.Background(), request("exec-next", port, 300))
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "COMPLETED" || next.AckLevel != "TRANSPORT_ONLY" {
		t.Fatalf("next result=%#v", next)
	}

	packet := make([]byte, 2048)
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadFromUDP(packet); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
	if _, _, err := conn.ReadFromUDP(packet); err == nil {
		t.Fatal("cancelled execution was replayed")
	}
}
