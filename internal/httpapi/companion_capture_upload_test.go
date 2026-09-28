package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/bulk"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

type captureUploadHarness struct {
	ctx        context.Context
	store      *store.Store
	auth       *companionauth.Service
	credential companionauth.RuntimeSessionCredential
	session    domain.CompanionRuntimeSession
	environment store.ExecutionEnvironmentManifest
	role       domain.MachineRole
	assignment domain.RoleAssignment
	runtimeSnapshotID string
	vault      *vault.Vault
	bulk       *bulk.Manager
	handler    http.Handler
}

func newCaptureUploadHarness(t *testing.T, mode bulk.Mode) *captureUploadHarness {
	t.Helper()
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = handle.Close() })
	stageStore := store.New(handle.DB, clock.Real{})
	auth := companionauth.New(stageStore, nil)
	credential := pairedRuntimeCredential(t, ctx, auth)
	session, err := stageStore.GetCompanionRuntimeSession(ctx, credential.SessionID)
	if err != nil { t.Fatal(err) }

	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Capture Upload Test", CreatedBy: "test"})
	if err != nil { t.Fatal(err) }
	environment, err := stageStore.CreateExecutionEnvironmentManifest(ctx, revision.ID, captureUploadManifest(), "test")
	if err != nil { t.Fatal(err) }
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "MAC-CAPTURE", DisplayName: "Capture Mac", Required: true,
	})
	if err != nil { t.Fatal(err) }
	assignment, err := stageStore.AssignMachineRole(ctx, role.ID, session.CompanionID)
	if err != nil { t.Fatal(err) }
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "test")
	if err != nil { t.Fatal(err) }

	v, err := vault.Open(t.TempDir(), stageStore)
	if err != nil { t.Fatal(err) }
	manager := bulk.New(func(context.Context) (bulk.Mode, error) { return mode, nil })
	handler := httpapi.New(httpapi.WithCompanionCaptureUpload(auth, stageStore, v, manager)).Handler()
	return &captureUploadHarness{
		ctx: ctx, store: stageStore, auth: auth, credential: credential, session: session,
		environment: environment, role: role, assignment: assignment, runtimeSnapshotID: runtimeSnapshot.ID,
		vault: v, bulk: manager, handler: handler,
	}
}

func (h *captureUploadHarness) ticket(t *testing.T, operation string, payload []byte) store.CompanionUploadTicketGrant {
	t.Helper()
	sum := sha256.Sum256(payload)
	grant, err := h.store.CreateCompanionUploadTicket(h.ctx, store.CreateCompanionUploadTicketParams{
		CompanionID: h.session.CompanionID,
		RuntimeSessionID: h.session.ID,
		OperationID: operation,
		EnvironmentManifestID: h.environment.ID,
		MachineRoleID: h.role.ID,
		RuntimeSnapshotID: h.runtimeSnapshotID,
		Purpose: store.CompanionUploadExecutionEnvironmentCapture,
		ExpectedContentHash: hex.EncodeToString(sum[:]),
		ExpectedSizeBytes: int64(len(payload)),
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil { t.Fatal(err) }
	return grant
}

func (h *captureUploadHarness) request(ticket store.CompanionUploadTicketGrant, body io.Reader, contentLength int64) *http.Request {
	path := "/api/v1/companion/capture-uploads/" + ticket.Ticket.ID
	req := httptest.NewRequest(http.MethodPut, path, body)
	req.RemoteAddr = "127.0.0.1:42001"
	req.Header.Set("Authorization", "StageCoreSession "+h.credential.Token)
	req.Header.Set("X-StageCore-Upload-Credential", ticket.Credential)
	req.ContentLength = contentLength
	return req
}

func TestCompanionCaptureUploadSuccessReplayAndCompletedTicketReference(t *testing.T) {
	h := newCaptureUploadHarness(t, bulk.ModeEdit)
	payload := []byte("full verified observable capture payload")
	ticket := h.ticket(t, "capture-http-success", payload)

	unauthorized := httptest.NewRequest(http.MethodPut, "/api/v1/companion/capture-uploads/"+ticket.Ticket.ID, bytes.NewReader(payload))
	unauthorized.RemoteAddr = "127.0.0.1:42000"
	unauthorized.Header.Set("X-StageCore-Upload-Credential", ticket.Credential)
	unauthorizedResult := httptest.NewRecorder()
	h.handler.ServeHTTP(unauthorizedResult, unauthorized)
	if unauthorizedResult.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d body=%s", unauthorizedResult.Code, unauthorizedResult.Body.String())
	}

	response := httptest.NewRecorder()
	h.handler.ServeHTTP(response, h.request(ticket, bytes.NewReader(payload), int64(len(payload))))
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q", response.Header().Get("Cache-Control"))
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil { t.Fatal(err) }
	if body["ticket_id"] != ticket.Ticket.ID || body["status"] != string(store.CompanionUploadTicketCompleted) {
		t.Fatalf("response=%v", body)
	}
	storedTicket, err := h.store.GetCompanionUploadTicket(h.ctx, ticket.Ticket.ID)
	if err != nil { t.Fatal(err) }
	if storedTicket.Status != store.CompanionUploadTicketCompleted || storedTicket.TerminalAt == nil {
		t.Fatalf("ticket=%+v", storedTicket)
	}
	object, err := h.store.GetVaultObject(h.ctx, ticket.Ticket.ExpectedContentHash)
	if err != nil { t.Fatal(err) }
	if object.SizeBytes != int64(len(payload)) { t.Fatalf("object=%+v", object) }
	removed, err := h.vault.RemoveObjectIfUnreferenced(h.ctx, object.ContentHash)
	if err != nil { t.Fatal(err) }
	if removed { t.Fatal("completed upload ticket did not protect its Vault object") }

	replay := httptest.NewRecorder()
	h.handler.ServeHTTP(replay, h.request(ticket, bytes.NewReader(payload), int64(len(payload))))
	if replay.Code != http.StatusConflict {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
}

func TestCompanionCaptureUploadShowAndIdentityFailuresStayActive(t *testing.T) {
	payload := []byte("capture identity payload")

	show := newCaptureUploadHarness(t, bulk.ModeShow)
	showTicket := show.ticket(t, "capture-http-show", payload)
	showRes := httptest.NewRecorder()
	show.handler.ServeHTTP(showRes, show.request(showTicket, bytes.NewReader(payload), int64(len(payload))))
	if showRes.Code != http.StatusLocked {
		t.Fatalf("SHOW status=%d body=%s", showRes.Code, showRes.Body.String())
	}
	showStored, err := show.store.GetCompanionUploadTicket(show.ctx, showTicket.Ticket.ID)
	if err != nil { t.Fatal(err) }
	if showStored.Status != store.CompanionUploadTicketActive { t.Fatalf("SHOW ticket=%+v", showStored) }

	edit := newCaptureUploadHarness(t, bulk.ModeEdit)
	lengthTicket := edit.ticket(t, "capture-http-length", payload)
	lengthRes := httptest.NewRecorder()
	edit.handler.ServeHTTP(lengthRes, edit.request(lengthTicket, bytes.NewReader(payload[:len(payload)-1]), int64(len(payload)-1)))
	if lengthRes.Code != http.StatusConflict {
		t.Fatalf("length status=%d body=%s", lengthRes.Code, lengthRes.Body.String())
	}
	lengthStored, err := edit.store.GetCompanionUploadTicket(edit.ctx, lengthTicket.Ticket.ID)
	if err != nil { t.Fatal(err) }
	if lengthStored.Status != store.CompanionUploadTicketActive { t.Fatalf("length ticket=%+v", lengthStored) }

	hashTicket := edit.ticket(t, "capture-http-hash", payload)
	wrong := bytes.Repeat([]byte{'x'}, len(payload))
	if bytes.Equal(wrong, payload) { t.Fatal("wrong hash fixture matches payload") }
	hashRes := httptest.NewRecorder()
	edit.handler.ServeHTTP(hashRes, edit.request(hashTicket, bytes.NewReader(wrong), int64(len(wrong))))
	if hashRes.Code != http.StatusUnprocessableEntity {
		t.Fatalf("hash status=%d body=%s", hashRes.Code, hashRes.Body.String())
	}
	hashStored, err := edit.store.GetCompanionUploadTicket(edit.ctx, hashTicket.Ticket.ID)
	if err != nil { t.Fatal(err) }
	if hashStored.Status != store.CompanionUploadTicketActive { t.Fatalf("hash ticket=%+v", hashStored) }
	if _, err := edit.store.GetVaultObject(edit.ctx, hashTicket.Ticket.ExpectedContentHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("hash mismatch committed object err=%v", err)
	}
}

type releaseAssignmentReader struct {
	source *bytes.Reader
	once sync.Once
	release func()
}

func (r *releaseAssignmentReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if n > 0 { r.once.Do(r.release) }
	return n, err
}

func TestCompanionCaptureUploadAuthorityLossRejectsCompletionAndRollsBack(t *testing.T) {
	h := newCaptureUploadHarness(t, bulk.ModeEdit)
	payload := []byte("authority-loss capture payload unique")
	ticket := h.ticket(t, "capture-http-authority-loss", payload)
	reader := &releaseAssignmentReader{
		source: bytes.NewReader(payload),
		release: func() {
			if err := h.store.ReleaseRoleAssignment(h.ctx, h.assignment.ID); err != nil {
				t.Errorf("release assignment: %v", err)
			}
		},
	}
	res := httptest.NewRecorder()
	h.handler.ServeHTTP(res, h.request(ticket, reader, int64(len(payload))))
	if res.Code != http.StatusConflict {
		t.Fatalf("authority-loss status=%d body=%s", res.Code, res.Body.String())
	}
	stored, err := h.store.GetCompanionUploadTicket(h.ctx, ticket.Ticket.ID)
	if err != nil { t.Fatal(err) }
	if stored.Status != store.CompanionUploadTicketCancelled || stored.TerminalAt == nil {
		t.Fatalf("authority-loss ticket=%+v", stored)
	}
	if _, err := h.store.GetVaultObject(h.ctx, ticket.Ticket.ExpectedContentHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("authority-loss object metadata err=%v", err)
	}
	path, err := h.vault.ObjectPath(ticket.Ticket.ExpectedContentHash)
	if err != nil { t.Fatal(err) }
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("authority-loss object file remains, stat err=%v", err)
	}
}

func captureUploadManifest() executionenv.Manifest {
	return executionenv.Manifest{
		SchemaVersion: executionenv.ManifestSchemaVersion,
		EnvironmentKey: "capture-upload",
		Name: "Capture upload environment",
		AdapterKey: "stagecore.adapter.vdmx",
		Application: executionenv.ApplicationRequirement{
			Key: "vdmx", Name: "VDMX", VersionConstraint: "8.x-tested",
			Hosts: []executionenv.HostRequirement{{OS: "darwin", Architecture: "arm64"}},
		},
	}
}
