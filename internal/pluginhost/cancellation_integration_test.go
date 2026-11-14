package pluginhost_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
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
	defer cancel()
	type executionResult struct {
		result pluginprotocol.ExecutionResult
		err    error
	}
	completed := make(chan executionResult, 1)
	go func() {
		result, err := host.Execute(ctx, request("exec-cancelled", port, 500))
		completed <- executionResult{result: result, err: err}
	}()
	deadline := time.NewTimer(2 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		select {
		case outcome := <-completed:
			t.Fatalf("first execution completed before cancellation: result=%+v err=%v", outcome.result, outcome.err)
		case <-ticker.C:
			if _, err := os.Stat(marker); err == nil {
				cancel()
				goto waitForCancellation
			}
		case <-deadline.C:
			cancel()
			t.Fatal("plugin fault fixture did not begin its first execution")
		}
	}

waitForCancellation:
	outcome := <-completed
	result, err := outcome.result, outcome.err
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
