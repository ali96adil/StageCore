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
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"golang.org/x/net/websocket"
)

func TestConnectedCompanionResyncsWhenPublishedSnapshotAdvances(t *testing.T) {
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
	companionID := "77777777-7777-4777-8777-777777777777"
	pairRuntimeCompanion(t, ctx, auth, companionID, publicKey)
	project, revision, role, _, first := runtimeCueFixture(t, ctx, s, companionID)

	server := httptest.NewServer(httpapi.New(
		httpapi.WithCompanionAuth(auth),
		httpapi.WithCompanionRuntime(runtime),
	).Handler())
	defer server.Close()

	credential := authenticateRuntimeCompanion(t, ctx, auth, companionID, privateKey)
	runtimeURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/companion/runtime"
	ws, err := dialRuntime(runtimeURL, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	if err := websocket.JSON.Send(ws, runtimeAgentHello(companionID, nil, nil, "UNKNOWN")); err != nil {
		t.Fatal(err)
	}
	var ready1 struct {
		Type              string `json:"type"`
		MachineRoleID     string `json:"machine_role_id"`
		RuntimeSnapshotID string `json:"runtime_snapshot_id"`
	}
	if err := websocket.JSON.Receive(ws, &ready1); err != nil {
		t.Fatal(err)
	}
	if ready1.Type != "session.ready" || ready1.MachineRoleID != role.ID || ready1.RuntimeSnapshotID != first.ID {
		t.Fatalf("initial session.ready=%+v", ready1)
	}
	if err := websocket.JSON.Send(ws, runtimeAgentHello(companionID, &role.ID, &first.ID, "READY")); err != nil {
		t.Fatal(err)
	}
	waitForRuntime(t, func() bool { return companionReady(ctx, s, companionID, first.ID) })

	second, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test-second")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.ProjectID != project.ID {
		t.Fatalf("second snapshot=%+v first=%+v", second, first)
	}

	if err := websocket.JSON.Send(ws, runtimeAgentHello(companionID, &role.ID, &first.ID, "READY")); err != nil {
		t.Fatal(err)
	}
	var ready2 struct {
		Type              string `json:"type"`
		MachineRoleID     string `json:"machine_role_id"`
		RuntimeSnapshotID string `json:"runtime_snapshot_id"`
	}
	if err := websocket.JSON.Receive(ws, &ready2); err != nil {
		t.Fatal(err)
	}
	if ready2.Type != "session.ready" || ready2.MachineRoleID != role.ID || ready2.RuntimeSnapshotID != second.ID {
		t.Fatalf("resync session.ready=%+v want snapshot=%s", ready2, second.ID)
	}

	if err := websocket.JSON.Send(ws, runtimeAgentHello(companionID, &role.ID, &second.ID, "READY")); err != nil {
		t.Fatal(err)
	}
	waitForRuntime(t, func() bool { return companionReady(ctx, s, companionID, second.ID) })
}
