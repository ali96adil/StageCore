package companionchannel_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/sessionmemory"
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
	project, _, _, _, runtimeSnapshot := runtimeCueFixture(t, ctx, s, companionID)
	timelineSession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Companion transport timeline")
	if err != nil {
		t.Fatal(err)
	}
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
	var detail sessionmemory.SessionDetail
	waitForRuntime(t, func() bool {
		var detailErr error
		detail, detailErr = sessionmemory.New(s).Detail(ctx, project.ID, timelineSession.ID)
		if detailErr != nil {
			return false
		}
		return countSessionEventCategory(detail.Events, "companion_health") == 3
	})
	transitions := make([]sessionmemory.SessionEventTrace, 0, 3)
	for _, event := range detail.Events {
		if event.Category == "companion_health" {
			transitions = append(transitions, event)
		}
	}
	if transitions[0].CompanionID != companionID || transitions[0].TransportState != "CONNECTED" || transitions[0].ConnectionGeneration != 1 ||
		transitions[1].TransportState != "DISCONNECTED" || transitions[1].TransitionReason != companionchannel.CompanionTransportDisconnectSocketClosed || transitions[1].ConnectionGeneration != 1 ||
		transitions[2].TransportState != "CONNECTED" || transitions[2].ConnectionGeneration != 2 {
		t.Fatalf("Companion Hub transport timeline=%+v", transitions)
	}
	runtime.Close()
	waitForRuntime(t, func() bool {
		var detailErr error
		detail, detailErr = sessionmemory.New(s).Detail(ctx, project.ID, timelineSession.ID)
		return detailErr == nil && countSessionEventCategory(detail.Events, "companion_health") == 4
	})
	transitions = transitions[:0]
	for _, event := range detail.Events {
		if event.Category == "companion_health" {
			transitions = append(transitions, event)
		}
	}
	if transitions[3].TransportState != "DISCONNECTED" ||
		transitions[3].TransitionReason != companionchannel.CompanionTransportDisconnectHubRestart ||
		transitions[3].ConnectionGeneration != 2 || runtime.IsConnected(companionID) {
		t.Fatalf("Hub restart transport transition=%+v connected=%v", transitions[3], runtime.IsConnected(companionID))
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
	project, _, role, _, runtimeSnapshot := runtimeCueFixture(t, ctx, s, companionID)
	timelineSession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Companion replacement timeline")
	if err != nil {
		t.Fatal(err)
	}
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
	var detail sessionmemory.SessionDetail
	waitForRuntime(t, func() bool {
		var detailErr error
		detail, detailErr = sessionmemory.New(s).Detail(ctx, project.ID, timelineSession.ID)
		if detailErr != nil {
			return false
		}
		return countSessionEventCategory(detail.Events, "companion_health") == 3
	})
	transitions := make([]sessionmemory.SessionEventTrace, 0, 3)
	for _, event := range detail.Events {
		if event.Category == "companion_health" {
			transitions = append(transitions, event)
		}
	}
	if transitions[0].TransportState != "CONNECTED" || transitions[0].ConnectionGeneration != 1 ||
		transitions[1].TransportState != "DISCONNECTED" || transitions[1].TransitionReason != companionchannel.CompanionTransportDisconnectConnectionReplaced || transitions[1].ConnectionGeneration != 1 ||
		transitions[2].TransportState != "CONNECTED" || transitions[2].ConnectionGeneration != 2 {
		t.Fatalf("replacement transport timeline=%+v", transitions)
	}
}

func TestCompanionTransportTimelineChangesProjectScopeWithRoleAssignment(t *testing.T) {
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
	companionID := "44444444-4444-4444-8444-444444444444"
	pairRuntimeCompanion(t, ctx, auth, companionID, publicKey)
	projectA, _, roleA, _, runtimeSnapshot := runtimeCueFixture(t, ctx, s, companionID)
	timelineSession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Companion Project scope change")
	if err != nil {
		t.Fatal(err)
	}
	assignmentA, err := s.GetActiveRoleAssignmentForCompanion(ctx, companionID)
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
	credential := authenticateRuntimeCompanion(t, ctx, auth, companionID, privateKey)
	agent := startRuntimeAgent(t, runtimeURL, credential.Token, companionID, memory)
	defer agent.close(t)
	waitForRuntime(t, func() bool {
		return runtime.IsConnected(companionID) && companionReady(ctx, s, companionID, runtimeSnapshot.ID)
	})

	if err := s.ReleaseRoleAssignment(ctx, assignmentA.ID); err != nil {
		t.Fatal(err)
	}
	projectB, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Companion next Project", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	roleB, err := s.CreateMachineRole(ctx, projectB.ID, store.CreateMachineRoleParams{
		RoleKey: "VIDEO-B", DisplayName: "Video B", RequiredCapabilities: []string{"local.echo"}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignMachineRole(ctx, roleB.ID, companionID); err != nil {
		t.Fatal(err)
	}
	if err := websocket.JSON.Send(agent.connection, runtimeAgentHello(companionID, &roleA.ID, &runtimeSnapshot.ID, "READY")); err != nil {
		t.Fatal(err)
	}
	var projectATimeline sessionmemory.SessionDetail
	waitForRuntime(t, func() bool {
		var detailErr error
		projectATimeline, detailErr = sessionmemory.New(s).Detail(ctx, projectA.ID, timelineSession.ID)
		if detailErr != nil {
			return false
		}
		return countSessionEventCategory(projectATimeline.Events, "companion_health") == 2
	})
	transitionsA := make([]sessionmemory.SessionEventTrace, 0, 2)
	for _, event := range projectATimeline.Events {
		if event.Category == "companion_health" {
			transitionsA = append(transitionsA, event)
		}
	}
	if transitionsA[0].TransportState != "CONNECTED" || transitionsA[1].TransportState != "DISCONNECTED" ||
		transitionsA[1].TransitionReason != companionchannel.CompanionTransportDisconnectProjectScopeChanged || !runtime.IsConnected(companionID) {
		t.Fatalf("Project A transport transitions=%+v connected=%v", transitionsA, runtime.IsConnected(companionID))
	}
	through := time.Now().UTC().Add(time.Minute)
	projectBEvents, truncated, err := s.ListProjectTransportEvents(ctx, projectB.ID, timelineSession.StartedAt, through)
	if err != nil || truncated || len(projectBEvents) != 1 ||
		projectBEvents[0].EventType != contracts.CompanionTransportConnectedEventType {
		t.Fatalf("Project B transport events=%+v truncated=%v err=%v", projectBEvents, truncated, err)
	}
	if projectBEvents[0].ProjectID != projectB.ID || projectBEvents[0].Source != "hub.companion_runtime" ||
		projectBEvents[0].RuntimeSnapshotID != "" {
		t.Fatalf("Project B event leaked incorrect scope or runtime evidence: %+v", projectBEvents[0])
	}
}

func countSessionEventCategory(events []sessionmemory.SessionEventTrace, category string) int {
	count := 0
	for _, event := range events {
		if event.Category == category {
			count++
		}
	}
	return count
}

func roleAssignmentState(ctx context.Context, s *store.Store, assignmentID string) domain.RoleAssignmentState {
	assignment, err := s.GetRoleAssignment(ctx, assignmentID)
	if err != nil {
		return ""
	}
	return assignment.State
}
