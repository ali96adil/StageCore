package companionchannel_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/store"
	"golang.org/x/net/websocket"
)

func TestRuntimeDisconnectAndReconnectUpdatePresenceWithoutReplay(t *testing.T) {
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

	privateKey, publicKey := runtimeDeviceKey(t)
	companionID := "22222222-2222-4222-8222-222222222222"
	pairRuntimeCompanion(t, ctx, auth, companionID, publicKey)
	_, _, _, _, runtimeSnapshot := runtimeCueFixture(t, ctx, s, companionID)
	assignment, err := s.GetActiveRoleAssignmentForCompanion(ctx, companionID)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(
		httpapi.WithCompanionAuth(auth),
		httpapi.WithCompanionRuntime(runtime),
	).Handler())
	defer server.Close()
	runtimeURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/companion/runtime"

	memory := &runtimeAgentMemory{seen: make(map[string]struct{})}
	firstCredential := authenticateRuntimeCompanion(t, ctx, auth, companionID, privateKey)
	firstAgent := startRuntimeAgent(t, runtimeURL, firstCredential.Token, companionID, memory)
	waitForRuntime(t, func() bool {
		return runtime.IsConnected(companionID) &&
			companionReady(ctx, s, companionID, runtimeSnapshot.ID) &&
			roleAssignmentState(ctx, s, assignment.ID) == domain.RoleReady
	})

	firstAgent.close(t)
	waitForRuntime(t, func() bool {
		state, err := s.GetCompanion(ctx, companionID)
		return err == nil && !runtime.IsConnected(companionID) &&
			state.Readiness == domain.CompanionReadinessOffline &&
			roleAssignmentState(ctx, s, assignment.ID) == domain.RoleOffline
	})
	if got := memory.count.Load(); got != 0 {
		t.Fatalf("disconnect replayed execution count=%d", got)
	}

	secondCredential := authenticateRuntimeCompanion(t, ctx, auth, companionID, privateKey)
	secondAgent := startRuntimeAgent(t, runtimeURL, secondCredential.Token, companionID, memory)
	defer secondAgent.close(t)
	waitForRuntime(t, func() bool {
		return runtime.IsConnected(companionID) &&
			companionReady(ctx, s, companionID, runtimeSnapshot.ID) &&
			roleAssignmentState(ctx, s, assignment.ID) == domain.RoleReady
	})
	if got := memory.count.Load(); got != 0 {
		t.Fatalf("reconnect replayed execution count=%d", got)
	}
}

func TestReplacingRuntimeConnectionDoesNotLetStaleSocketForceOffline(t *testing.T) {
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

	privateKey, publicKey := runtimeDeviceKey(t)
	companionID := "33333333-3333-4333-8333-333333333333"
	pairRuntimeCompanion(t, ctx, auth, companionID, publicKey)
	_, _, role, _, runtimeSnapshot := runtimeCueFixture(t, ctx, s, companionID)
	assignment, err := s.GetActiveRoleAssignmentForCompanion(ctx, companionID)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(
		httpapi.WithCompanionAuth(auth),
		httpapi.WithCompanionRuntime(runtime),
	).Handler())
	defer server.Close()
	runtimeURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/companion/runtime"

	memory := &runtimeAgentMemory{seen: make(map[string]struct{})}
	firstCredential := authenticateRuntimeCompanion(t, ctx, auth, companionID, privateKey)
	firstAgent := startRuntimeAgent(t, runtimeURL, firstCredential.Token, companionID, memory)
	waitForRuntime(t, func() bool {
		return runtime.IsConnected(companionID) && companionReady(ctx, s, companionID, runtimeSnapshot.ID)
	})

	secondCredential := authenticateRuntimeCompanion(t, ctx, auth, companionID, privateKey)
	secondConnection, err := dialRuntime(runtimeURL, secondCredential.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer secondConnection.Close()
	if err := websocket.JSON.Send(secondConnection, runtimeAgentHello(companionID, nil, nil, "UNKNOWN")); err != nil {
		t.Fatal(err)
	}
	var ready struct {
		Type              string `json:"type"`
		MachineRoleID     string `json:"machine_role_id"`
		RuntimeSnapshotID string `json:"runtime_snapshot_id"`
	}
	if err := websocket.JSON.Receive(secondConnection, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Type != "session.ready" || ready.MachineRoleID != role.ID || ready.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf("replacement session.ready=%#v", ready)
	}
	firstAgent.waitClosed(t)

	state, err := s.GetCompanion(ctx, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Readiness == domain.CompanionReadinessOffline {
		t.Fatal("stale replaced connection forced the current Companion OFFLINE")
	}
	if !runtime.IsConnected(companionID) {
		t.Fatal("replacement runtime connection was removed by stale socket close")
	}

	if err := websocket.JSON.Send(secondConnection, runtimeAgentHello(companionID, &role.ID, &runtimeSnapshot.ID, "READY")); err != nil {
		t.Fatal(err)
	}
	waitForRuntime(t, func() bool {
		return companionReady(ctx, s, companionID, runtimeSnapshot.ID) &&
			roleAssignmentState(ctx, s, assignment.ID) == domain.RoleReady
	})
	if got := memory.count.Load(); got != 0 {
		t.Fatalf("connection replacement replayed execution count=%d", got)
	}
}

func roleAssignmentState(ctx context.Context, s *store.Store, assignmentID string) domain.RoleAssignmentState {
	assignment, err := s.GetRoleAssignment(ctx, assignmentID)
	if err != nil {
		return ""
	}
	return assignment.State
}
