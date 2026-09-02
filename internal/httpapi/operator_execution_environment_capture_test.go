package httpapi

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
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestOperatorExecutionEnvironmentCaptureStreamsIntoVaultAndPromotesManifest(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	v, err := vault.Open(t.TempDir(), stageStore)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(
		WithOperatorExecutionEnvironments(h.auth, stageStore),
		WithOperatorExecutionEnvironmentCapture(h.auth, stageStore, v),
	).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Capture API", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(ctx, revision.ID, testVDMXExecutionEnvironmentManifest("video-main"), owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + environment.ID + "/assets/workspace"
	payload := []byte("stagecore-vdmx-workspace-capture")

	unauthenticated := httptest.NewRequest(http.MethodPost, base+"/capture", bytes.NewReader(payload))
	unauthenticated.RemoteAddr = "127.0.0.1:14100"
	unauthenticatedRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRes, unauthenticated)
	if unauthenticatedRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated capture status=%d body=%s", unauthenticatedRes.Code, unauthenticatedRes.Body.String())
	}

	missingCSRF := httptest.NewRequest(http.MethodPost, base+"/capture", bytes.NewReader(payload))
	missingCSRF.RemoteAddr = "127.0.0.1:14101"
	missingCSRF.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	missingCSRFRes := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFRes, missingCSRF)
	if missingCSRFRes.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d body=%s", missingCSRFRes.Code, missingCSRFRes.Body.String())
	}

	badAsset := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, base[:len(base)-len("workspace")]+"missing/capture", payload)
	badAssetRes := httptest.NewRecorder()
	handler.ServeHTTP(badAssetRes, badAsset)
	if badAssetRes.Code != http.StatusNotFound {
		t.Fatalf("bad asset status=%d body=%s", badAssetRes.Code, badAssetRes.Body.String())
	}

	captureReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, base+"/capture", payload)
	captureRes := httptest.NewRecorder()
	handler.ServeHTTP(captureRes, captureReq)
	if captureRes.Code != http.StatusOK {
		t.Fatalf("capture status=%d body=%s", captureRes.Code, captureRes.Body.String())
	}
	var response struct {
		ExecutionEnvironment executionEnvironmentView `json:"execution_environment"`
		VaultObject struct {
			ContentHash string `json:"content_hash"`
			SizeBytes   int64  `json:"size_bytes"`
		} `json:"vault_object"`
	}
	if err := json.Unmarshal(captureRes.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	wantHash := hex.EncodeToString(sum[:])
	if response.VaultObject.ContentHash != wantHash || response.VaultObject.SizeBytes != int64(len(payload)) {
		t.Fatalf("Vault response=%+v want hash=%s size=%d", response.VaultObject, wantHash, len(payload))
	}
	if len(response.ExecutionEnvironment.Manifest.Assets) != 1 {
		t.Fatalf("captured manifest assets=%+v", response.ExecutionEnvironment.Manifest.Assets)
	}
	asset := response.ExecutionEnvironment.Manifest.Assets[0]
	if asset.CapturePolicy != "CONTENT_BOUND" || asset.ContentHash != wantHash || asset.SizeBytes == nil || *asset.SizeBytes != int64(len(payload)) {
		t.Fatalf("captured asset=%+v", asset)
	}

	file, object, err := v.OpenObject(ctx, wantHash)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, payload) || object.SizeBytes != int64(len(payload)) {
		t.Fatalf("stored payload=%q object=%+v", stored, object)
	}

	statusReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodGet, base+"/vault-status", nil)
	statusRes := httptest.NewRecorder()
	handler.ServeHTTP(statusRes, statusReq)
	if statusRes.Code != http.StatusOK {
		t.Fatalf("Vault status=%d body=%s", statusRes.Code, statusRes.Body.String())
	}
	var status struct {
		VaultAvailable bool   `json:"vault_available"`
		Reason         string `json:"reason"`
		ContentHash    string `json:"content_hash"`
	}
	if err := json.Unmarshal(statusRes.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.VaultAvailable || status.Reason != "AVAILABLE" || status.ContentHash != wantHash {
		t.Fatalf("Vault status payload=%+v", status)
	}

	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	blockedPayload := []byte("must-not-enter-vault-after-freeze")
	blockedSum := sha256.Sum256(blockedPayload)
	blockedHash := hex.EncodeToString(blockedSum[:])
	blockedReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, base+"/capture", blockedPayload)
	blockedRes := httptest.NewRecorder()
	handler.ServeHTTP(blockedRes, blockedReq)
	if blockedRes.Code != http.StatusConflict {
		t.Fatalf("frozen capture status=%d body=%s", blockedRes.Code, blockedRes.Body.String())
	}
	if _, err := stageStore.GetVaultObject(ctx, blockedHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("frozen capture unexpectedly registered Vault object err=%v", err)
	}
}

func TestOperatorExecutionEnvironmentCaptureSHOWLockRejectsBeforeVaultImport(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	v, err := vault.Open(t.TempDir(), stageStore)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Capture SHOW lock", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(ctx, revision.ID, testVDMXExecutionEnvironmentManifest("video-main"), owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Cue One", OrderIndex: 0,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	show, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "active show")
	if err != nil {
		t.Fatal(err)
	}
	defer stageStore.EndSession(ctx, show.ID, domain.SessionCompleted)

	handler := New(WithOperatorExecutionEnvironmentCapture(h.auth, stageStore, v)).Handler()
	path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + environment.ID + "/assets/workspace/capture"
	payload := []byte("show-blocked-capture")
	sum := sha256.Sum256(payload)
	wantHash := hex.EncodeToString(sum[:])
	req := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, path, payload)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusLocked {
		t.Fatalf("SHOW capture status=%d body=%s", res.Code, res.Body.String())
	}
	var blocked map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &blocked); err != nil {
		t.Fatal(err)
	}
	if blocked["error_code"] != "SHOW_CONFIGURATION_LOCKED" {
		t.Fatalf("SHOW capture error=%v", blocked)
	}
	if _, err := stageStore.GetVaultObject(ctx, wantHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SHOW capture unexpectedly registered Vault object err=%v", err)
	}
}
