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
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
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

	changed := testVDMXExecutionEnvironmentManifest("video-main")
	changed.Assets[0].Locator = "/Users/show/Replacement.vdmx5"
	stale := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, base+"/capture", payload)
	stale.Body = &firmwareUploadRevokeReader{ReadCloser: stale.Body, revoke: func() {
		if _, err := stageStore.UpdateExecutionEnvironmentManifest(ctx, environment.ID, changed); err != nil {
			t.Fatal(err)
		}
	}}
	staleResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleResponse, stale)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale capture status=%d body=%s", staleResponse.Code, staleResponse.Body.String())
	}
	if _, err := stageStore.UpdateExecutionEnvironmentManifest(ctx, environment.ID, environment.Manifest); err != nil {
		t.Fatal(err)
	}

	captureReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, base+"/capture", payload)
	captureRes := httptest.NewRecorder()
	handler.ServeHTTP(captureRes, captureReq)
	if captureRes.Code != http.StatusOK {
		t.Fatalf("capture status=%d body=%s", captureRes.Code, captureRes.Body.String())
	}
	var response struct {
		ExecutionEnvironment executionEnvironmentView `json:"execution_environment"`
		VaultObject          struct {
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

	committedProject, committedRevision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Capture response revocation", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	committedEnvironment, err := stageStore.CreateExecutionEnvironmentManifest(ctx, committedRevision.ID, testVDMXExecutionEnvironmentManifest("response-revoked"), owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	revoked := false
	revokedHandler := New(withOperatorExecutionEnvironmentCapture(h.auth, stageStore, v, executionEnvironmentCaptureHooks{
		BeforeResponse: func() {
			revoked = true
			if err := h.auth.Logout(ctx, owner.Token); err != nil {
				t.Errorf("revoke Owner after environment asset commit: %v", err)
			}
		},
	})).Handler()
	committedPayload := []byte("capture committed before response revocation")
	committedDigest := sha256.Sum256(committedPayload)
	committedHash := hex.EncodeToString(committedDigest[:])
	committedPath := "/api/v1/projects/" + committedProject.ID + "/revisions/" + committedRevision.ID + "/execution-environments/" + committedEnvironment.ID + "/assets/workspace/capture"
	committedRequest := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, committedPath, committedPayload)
	committedResponse := httptest.NewRecorder()
	revokedHandler.ServeHTTP(committedResponse, committedRequest)
	if !revoked {
		t.Fatal("test did not revoke Owner after execution environment capture commit")
	}
	if committedResponse.Code != http.StatusForbidden || !strings.Contains(committedResponse.Body.String(), `"effect_may_be_committed":true`) ||
		!strings.Contains(committedResponse.Body.String(), `"retryable":false`) || strings.Contains(committedResponse.Body.String(), committedHash) {
		t.Fatalf("revoked Owner received committed capture result: status=%d body=%s", committedResponse.Code, committedResponse.Body.String())
	}
	committedManifest, err := stageStore.GetExecutionEnvironmentManifest(ctx, committedEnvironment.ID)
	if err != nil || committedManifest.Manifest.Assets[0].ContentHash != committedHash {
		t.Fatalf("capture should remain committed after response revocation: manifest=%+v err=%v", committedManifest, err)
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

func TestScopedOperatorCanUploadProjectMediaWithExplicitGrant(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "media-upload-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	operatorLogin, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Scoped media upload"})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(ctx, revision.ID, testVDMXExecutionEnvironmentManifest("scoped-media"), owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.ProjectEdit, projectaccess.MediaUpload,
	}); err != nil {
		t.Fatal(err)
	}
	objects, err := vault.Open(t.TempDir(), stageStore)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(
		WithOperatorExecutionEnvironmentCapture(h.auth, stageStore, objects),
		WithProjectRouteQualification(access),
	).Handler()
	path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + environment.ID + "/assets/workspace/capture"
	const payload = "project scoped operator media"
	req := authenticatedExecutionEnvironmentRequest(t, operatorLogin.Token, operatorLogin.CSRFToken, http.MethodPost, path, []byte(payload))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("scoped Operator upload returned %d: %s", response.Code, response.Body.String())
	}
}

func TestScopedOperatorCaptureStopsWhenGrantIsRevokedDuringUpload(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := f.projects.CreateExecutionEnvironmentManifest(
		ctx, project.CurrentRevisionID, testVDMXExecutionEnvironmentManifest("revoke-scoped-media"), f.owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, project.ID, []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.ProjectEdit, projectaccess.MediaUpload,
	}); err != nil {
		t.Fatal(err)
	}
	objects, err := vault.Open(t.TempDir(), f.projects)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(
		WithOperatorExecutionEnvironmentCapture(f.h.auth, f.projects, objects),
		WithProjectRouteQualification(f.access),
	).Handler()
	path := "/api/v1/projects/" + project.ID + "/revisions/" + project.CurrentRevisionID + "/execution-environments/" + environment.ID + "/assets/workspace/capture"
	payload := strings.Repeat("scoped capture payload", 4096)
	req := authenticatedExecutionEnvironmentRequest(t, f.operator.Token, f.operator.CSRFToken, http.MethodPost, path, []byte(payload))
	source := strings.NewReader(payload)
	req.Body = io.NopCloser(&revokeDuringRead{Reader: source, revoke: func() {
		if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, project.ID, nil); err != nil {
			t.Fatal(err)
		}
	}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code == http.StatusOK || source.Len() == 0 {
		t.Fatalf("grant revocation did not stop upload: status=%d unread=%d body=%s", response.Code, source.Len(), response.Body.String())
	}
	digest := sha256.Sum256([]byte(payload))
	if _, err := f.projects.GetVaultObject(ctx, hex.EncodeToString(digest[:])); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revoked scoped upload registered a Vault object: %v", err)
	}
}

func TestLegacyBrowserCaptureStopsReadingAfterSessionRevocation(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := f.projects.CreateExecutionEnvironmentManifest(
		ctx, project.CurrentRevisionID, testVDMXExecutionEnvironmentManifest("revoked"), f.owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(t.TempDir(), f.projects)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorExecutionEnvironmentCapture(f.h.auth, f.projects, v)).Handler()
	path := "/api/v1/projects/" + project.ID + "/revisions/" + project.CurrentRevisionID + "/execution-environments/" + environment.ID + "/assets/workspace/capture"
	payload := bytes.Repeat([]byte("capture"), 100000)
	req := authenticatedMutationRequest(t, http.MethodPost, path, payload, f.owner.Token, f.owner.CSRFToken)
	source := bytes.NewReader(payload)
	req.Body = &firmwareUploadRevokeReader{ReadCloser: io.NopCloser(source), revoke: func() {
		if err := f.h.auth.Logout(ctx, f.owner.Token); err != nil {
			t.Fatal(err)
		}
	}}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden || source.Len() == 0 {
		t.Fatalf("revoked capture returned %d with %d bytes unread: %s", res.Code, source.Len(), res.Body)
	}
	digest := sha256.Sum256(payload)
	if _, err := f.projects.GetVaultObject(ctx, hex.EncodeToString(digest[:])); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revoked capture registered a Vault object: %v", err)
	}
}
