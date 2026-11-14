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
	"net/url"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestScopedProjectMediaUploadRequiresExplicitLiveGrantAndStoresManagedContent(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	objects, err := vault.Open(t.TempDir(), projects)
	if err != nil {
		t.Fatal(err)
	}
	projectA, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Upload rehearsal"})
	if err != nil {
		t.Fatal(err)
	}
	projectB, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Other rehearsal"})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "project-file-uploader", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(
		WithScopedProjectFilesRead(h.auth, access),
		WithScopedProjectMediaUpload(h.auth, access, projects, objects),
		WithScopedProjectMediaDownload(h.auth, access, projects, objects),
	).Handler()
	makeUpload := func(projectID, filename, assetName string, body []byte) *http.Request {
		t.Helper()
		req := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken,
			http.MethodPost, "/api/v1/projects/"+projectID+"/files", body)
		req.Header.Set("X-Project-File-Name", filename)
		if assetName != "" {
			req.Header.Set("X-Project-Asset-Name", assetName)
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		return req
	}
	set := func(projectID string, permissions ...projectaccess.Permission) {
		t.Helper()
		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, projectID, permissions); err != nil {
			t.Fatal(err)
		}
	}
	upload := func(req *http.Request) *httptest.ResponseRecorder {
		t.Helper()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	// The endpoint itself is opt-in and does not infer project upload rights
	// from the user's global OPERATOR role or from media.read.
	noOptIn := New().Handler()
	missing := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectA.ID+"/files", strings.NewReader("x"))
	missing.RemoteAddr = "127.0.0.1:41000"
	missingRes := httptest.NewRecorder()
	noOptIn.ServeHTTP(missingRes, missing)
	if missingRes.Code != http.StatusNotFound {
		t.Fatalf("unconfigured upload route status=%d", missingRes.Code)
	}
	set(projectA.ID, projectaccess.ProjectView, projectaccess.MediaRead)
	if got := upload(makeUpload(projectA.ID, "clip.wav", "Clip", []byte("audio"))); got.Code != http.StatusNotFound {
		t.Fatalf("MediaRead without MediaUpload created a file: %d %s", got.Code, got.Body.String())
	}
	set(projectA.ID, projectaccess.ProjectView, projectaccess.MediaRead, projectaccess.MediaUpload)
	badType := makeUpload(projectA.ID, "invalid.bin", "Invalid", []byte("body"))
	badType.Header.Set("Content-Type", "not a media type")
	if got := upload(badType); got.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("malformed media type status=%d body=%s", got.Code, got.Body.String())
	}
	mismatchedType := makeUpload(projectA.ID, "invalid.wav", "Invalid", []byte("body"))
	mismatchedType.Header.Set("Content-Type", "text/plain")
	if got := upload(mismatchedType); got.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("filename/MIME category mismatch status=%d body=%s", got.Code, got.Body.String())
	}
	badName := makeUpload(projectA.ID, "%ZZ", "Invalid", []byte("body"))
	if got := upload(badName); got.Code != http.StatusBadRequest {
		t.Fatalf("malformed encoded filename status=%d body=%s", got.Code, got.Body.String())
	}

	missingCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectA.ID+"/files", strings.NewReader("blocked"))
	missingCSRF.RemoteAddr = "127.0.0.1:41000"
	missingCSRF.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	missingCSRF.Header.Set("X-Project-File-Name", "blocked.bin")
	missingCSRF.Header.Set("Content-Type", "application/octet-stream")
	if got := upload(missingCSRF); got.Code != http.StatusForbidden {
		t.Fatalf("upload without CSRF status=%d body=%s", got.Code, got.Body.String())
	}

	payload := []byte("StageCore project media bytes")
	req := makeUpload(projectA.ID, url.PathEscape("../../opening cue.wav"), url.PathEscape("Opening cue"), payload)
	response := upload(req)
	if response.Code != http.StatusCreated || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("upload status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	var result struct {
		AssetID string `json:"media_asset_id"`
		Version string `json:"content_version_id"`
		Hash    string `json:"content_hash"`
		Size    int64  `json:"size_bytes"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	wantHash := hex.EncodeToString(sum[:])
	if result.AssetID == "" || result.Version == "" || result.Hash != wantHash || result.Size != int64(len(payload)) {
		t.Fatalf("upload result=%+v expected sha=%s size=%d", result, wantHash, len(payload))
	}
	file, object, err := objects.OpenObject(ctx, result.Hash)
	if err != nil {
		t.Fatal(err)
	}
	var stored bytes.Buffer
	_, copyErr := stored.ReadFrom(file)
	_ = file.Close()
	if copyErr != nil || !bytes.Equal(stored.Bytes(), payload) || object.SizeBytes != int64(len(payload)) {
		t.Fatalf("Vault content=%q object=%+v copy error=%v", stored.Bytes(), object, copyErr)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectA.ID+"/files?page_size=100", nil)
	list.RemoteAddr = "127.0.0.1:41000"
	list.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"upload_available":true`) ||
		!strings.Contains(listResponse.Body.String(), `"read_only":false`) ||
		!strings.Contains(listResponse.Body.String(), `"original_filename":"opening cue.wav"`) ||
		!strings.Contains(listResponse.Body.String(), `"media_asset_name":"Opening cue"`) {
		t.Fatalf("uploaded Project Files not listed with safe name: %d %s", listResponse.Code, listResponse.Body.String())
	}
	download := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectA.ID+"/files/"+result.Version+"/download", nil)
	download.RemoteAddr = "127.0.0.1:41000"
	download.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	downloadResponse := httptest.NewRecorder()
	handler.ServeHTTP(downloadResponse, download)
	if downloadResponse.Code != http.StatusOK || !bytes.Equal(downloadResponse.Body.Bytes(), payload) {
		t.Fatalf("uploaded file download status=%d body=%q", downloadResponse.Code, downloadResponse.Body.Bytes())
	}
	if got := upload(makeUpload(projectB.ID, "cross-project.bin", "Cross Project", payload)); got.Code != http.StatusNotFound {
		t.Fatalf("Project A grant uploaded into Project B: %d %s", got.Code, got.Body.String())
	}
	if _, err := projects.GetMediaContentVersion(ctx, result.Version); err != nil {
		t.Fatalf("registered media version was not durable: %v", err)
	}
}

func TestScopedProjectMediaUploadStopsOnRevocationAndBoundsRequest(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	objects, err := vault.Open(t.TempDir(), projects)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "revocable-file-uploader", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Revoked upload"})
	if err != nil {
		t.Fatal(err)
	}
	set := func(permissions ...projectaccess.Permission) {
		t.Helper()
		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, permissions); err != nil {
			t.Fatal(err)
		}
	}
	set(projectaccess.ProjectView, projectaccess.MediaRead, projectaccess.MediaUpload)
	handler := New(WithScopedProjectMediaUpload(h.auth, access, projects, objects)).Handler()
	payload := strings.Repeat("private media", 4096)
	sum := sha256.Sum256([]byte(payload))
	request := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodPost,
		"/api/v1/projects/"+project.ID+"/files", []byte(payload))
	request.Header.Set("X-Project-File-Name", "revoked.wav")
	request.Header.Set("Content-Type", "audio/wav")
	request.Body = io.NopCloser(&revokeDuringRead{Reader: strings.NewReader(payload), revoke: func() { set() }})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code == http.StatusCreated || !strings.Contains(response.Body.String(), "PROJECT_NOT_FOUND") {
		t.Fatalf("grant revocation failed to stop upload: status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := projects.GetVaultObject(ctx, hex.EncodeToString(sum[:])); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revoked upload left a registered Vault object: %v", err)
	}

	set(projectaccess.ProjectView, projectaccess.MediaRead, projectaccess.MediaUpload)
	tooLarge := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodPost,
		"/api/v1/projects/"+project.ID+"/files", []byte("not actually large"))
	tooLarge.Header.Set("X-Project-File-Name", "oversized.bin")
	tooLarge.Header.Set("Content-Type", "application/octet-stream")
	tooLarge.ContentLength = maxProjectFileUploadBytes + 1
	largeResponse := httptest.NewRecorder()
	handler.ServeHTTP(largeResponse, tooLarge)
	if largeResponse.Code != http.StatusRequestEntityTooLarge || strings.Contains(largeResponse.Body.String(), "effect_may_have_been_committed") {
		t.Fatalf("oversized upload status=%d body=%s", largeResponse.Code, largeResponse.Body.String())
	}
}
