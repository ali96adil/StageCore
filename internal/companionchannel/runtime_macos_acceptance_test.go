//go:build darwin && macos_companion_acceptance

package companionchannel_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestRealMacOSCompanionReplacementAcceptance(t *testing.T) {
	companionBinary := strings.TrimSpace(os.Getenv("STAGECORE_COMPANION_BIN"))
	if companionBinary == "" {
		t.Fatal("STAGECORE_COMPANION_BIN is required for macOS Companion acceptance")
	}
	if _, err := os.Stat(companionBinary); err != nil {
		t.Fatalf("Companion binary unavailable: %v", err)
	}

	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()

	s := store.New(handle.DB, clock.Real{})
	auth := companionauth.New(s, nil)
	runtime := companionchannel.NewRuntime(s, auth)
	defer runtime.Close()

	server := httptest.NewServer(httpapi.New(
		httpapi.WithCompanionAuth(auth),
		httpapi.WithCompanionRuntime(runtime),
	).Handler())
	defer server.Close()
	runtimeURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/companion/runtime"

	runID := time.Now().UTC().UnixNano()
	macA := startRealMacOSCompanion(
		t,
		companionBinary,
		server.URL+"/",
		runtimeURL,
		"Video Mac A",
		fmt.Sprintf("com.stagecore.companion.acceptance.%d.a", runID),
	)
	defer macA.stop()
	if _, err := auth.ApprovePairing(
		ctx,
		macA.pairingRequestID,
		macA.pairingCode,
		companionauth.Approval{Actor: "acceptance.operator", Authorized: true},
	); err != nil {
		t.Fatalf("approve first Companion: %v", err)
	}

	project, _, role, cue, runtimeSnapshot := runtimeCueFixture(t, ctx, s, macA.companionID)
	waitForRealCompanion(t, func() bool {
		return runtime.IsConnected(macA.companionID) && companionReady(ctx, s, macA.companionID, runtimeSnapshot.ID)
	})

	registry := capability.NewRegistry()
	if err := registry.RegisterTargetType(
		companion.MachineRoleLogicalType,
		companion.NewForwarder(s, runtime, 5*time.Second, nil),
	); err != nil {
		t.Fatal(err)
	}
	engine := cueengine.NewWithExecutor(s, registry)

	firstSession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "real macOS Companion A")
	if err != nil {
		t.Fatal(err)
	}
	firstResult := engine.ExecuteCueGo(ctx, firstSession.ID, runtimeCueCommand(t, project.ID, runtimeSnapshot.ID))
	if firstResult.Status != contracts.CommandCompleted {
		t.Fatalf("first real Companion result=%#v", firstResult)
	}
	firstExecutions, err := s.ListCueExecutions(ctx, firstSession.ID)
	if err != nil || len(firstExecutions) != 1 || firstExecutions[0].CueID != cue.ID {
		t.Fatalf("first real Companion Cue=%#v err=%v want=%s", firstExecutions, err, cue.ID)
	}
	assertRecordedCompanionResult(t, ctx, s, firstSession.ID, 1)

	assignmentA, err := s.GetActiveRoleAssignment(ctx, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	macA.stop()
	waitForRealCompanion(t, func() bool { return !runtime.IsConnected(macA.companionID) })
	if err := s.ReleaseRoleAssignment(ctx, assignmentA.ID); err != nil {
		t.Fatal(err)
	}

	macB := startRealMacOSCompanion(
		t,
		companionBinary,
		server.URL+"/",
		runtimeURL,
		"Video Mac B",
		fmt.Sprintf("com.stagecore.companion.acceptance.%d.b", runID),
	)
	defer macB.stop()
	if macB.companionID == macA.companionID {
		t.Fatal("replacement Companion reused first device identity")
	}
	if _, err := auth.ApprovePairing(
		ctx,
		macB.pairingRequestID,
		macB.pairingCode,
		companionauth.Approval{Actor: "acceptance.operator", Authorized: true},
	); err != nil {
		t.Fatalf("approve replacement Companion: %v", err)
	}
	assignmentB, err := s.AssignMachineRole(ctx, role.ID, macB.companionID)
	if err != nil {
		t.Fatalf("assign replacement Companion: %v", err)
	}
	if assignmentB.CompanionID != macB.companionID {
		t.Fatalf("replacement assignment=%s want=%s", assignmentB.CompanionID, macB.companionID)
	}
	waitForRealCompanion(t, func() bool {
		return runtime.IsConnected(macB.companionID) && companionReady(ctx, s, macB.companionID, runtimeSnapshot.ID)
	})

	// Merely pairing/connecting the replacement must not replay prior work.
	if executions, err := s.ListCueExecutions(ctx, firstSession.ID); err != nil || len(executions) != 1 {
		t.Fatalf("replacement connection changed prior execution history: %#v err=%v", executions, err)
	}

	secondSession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "real macOS Companion B")
	if err != nil {
		t.Fatal(err)
	}
	secondResult := engine.ExecuteCueGo(ctx, secondSession.ID, runtimeCueCommand(t, project.ID, runtimeSnapshot.ID))
	if secondResult.Status != contracts.CommandCompleted {
		t.Fatalf("replacement real Companion result=%#v", secondResult)
	}
	secondExecutions, err := s.ListCueExecutions(ctx, secondSession.ID)
	if err != nil || len(secondExecutions) != 1 || secondExecutions[0].CueID != cue.ID {
		t.Fatalf("replacement real Companion Cue=%#v err=%v want=%s", secondExecutions, err, cue.ID)
	}
	assertRecordedCompanionResult(t, ctx, s, secondSession.ID, 1)

	if firstSession.RuntimeSnapshotID != secondSession.RuntimeSnapshotID || secondSession.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf(
			"real Companion replacement changed Runtime Snapshot: first=%s second=%s expected=%s",
			firstSession.RuntimeSnapshotID,
			secondSession.RuntimeSnapshotID,
			runtimeSnapshot.ID,
		)
	}
}

type realMacOSCompanion struct {
	cmd              *exec.Cmd
	lines            <-chan string
	stderr           *bytes.Buffer
	companionID      string
	pairingRequestID string
	pairingCode      string
	waited           bool
}

func startRealMacOSCompanion(
	t *testing.T,
	binary, hubAPIURL, runtimeURL, displayName, identityService string,
) *realMacOSCompanion {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "companion.json")
	cmd := exec.Command(
		binary,
		"--config", configPath,
		"--hub-api", hubAPIURL,
		"--hub-runtime", runtimeURL,
		"--display-name", displayName,
	)
	cmd.Env = append(
		os.Environ(),
		"STAGECORE_COMPANION_ALLOW_INSECURE_LOOPBACK_FOR_TESTING=1",
		"STAGECORE_COMPANION_IDENTITY_SERVICE="+identityService,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start macOS Companion: %v", err)
	}

	lines := make(chan string, 32)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	process := &realMacOSCompanion{cmd: cmd, lines: lines, stderr: stderr}

	startLine := process.waitForPrefix(t, "StageCore Companion ")
	fields := strings.Fields(startLine)
	if len(fields) < 4 || fields[0] != "StageCore" || fields[1] != "Companion" {
		process.stop()
		t.Fatalf("invalid Companion startup line %q", startLine)
	}
	process.companionID = fields[2]
	process.pairingRequestID = strings.TrimSpace(strings.TrimPrefix(
		process.waitForPrefix(t, "Pairing request: "),
		"Pairing request: ",
	))
	process.pairingCode = strings.TrimSpace(strings.TrimPrefix(
		process.waitForPrefix(t, "Pairing code: "),
		"Pairing code: ",
	))
	if process.companionID == "" || process.pairingRequestID == "" || process.pairingCode == "" {
		process.stop()
		t.Fatal("Companion did not expose complete pairing bootstrap state")
	}
	return process
}

func (process *realMacOSCompanion) waitForPrefix(t *testing.T, prefix string) string {
	t.Helper()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-process.lines:
			if !ok {
				err := process.wait()
				t.Fatalf("Companion exited while waiting for %q: err=%v stderr=%s", prefix, err, process.stderr.String())
			}
			if strings.HasPrefix(line, prefix) {
				return line
			}
		case <-timer.C:
			process.stop()
			t.Fatalf("timeout waiting for Companion output %q; stderr=%s", prefix, process.stderr.String())
		}
	}
}

func (process *realMacOSCompanion) wait() error {
	if process == nil || process.cmd == nil || process.waited {
		return nil
	}
	process.waited = true
	return process.cmd.Wait()
}

func (process *realMacOSCompanion) stop() {
	if process == nil || process.cmd == nil || process.waited {
		return
	}
	if process.cmd.Process != nil {
		_ = process.cmd.Process.Kill()
	}
	_ = process.wait()
}

func waitForRealCompanion(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("real macOS Companion runtime condition did not become true")
}
