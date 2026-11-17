package httpapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/bulk"
	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/stageaudio"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

// This production-shaped option composition ensures the staged project
// filters, global route policy, media/capsule paths, StageAudio, Companion,
// and device metadata do not regress when installed in the same Router.
func TestSC024UnifiedScopedACLAssemblyRoleAndProjectMatrix(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	projectA, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Unified ACL A"})
	if err != nil {
		t.Fatal(err)
	}
	projectB, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Unified ACL B"})
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "unified-acl-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.CreateUser(ctx, "unified-acl-viewer", h.password, userauth.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	operatorLogin, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerLogin, err := h.auth.Login(ctx, viewer.Username, h.password, "127.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	operatorGrants := []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.CueEdit, projectaccess.MediaRead,
		projectaccess.CapsuleExport, projectaccess.AudioControl,
		projectaccess.RehearsalControl, projectaccess.DeviceConfigure,
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, projectA.ID, operatorGrants); err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, projectA.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.MediaRead}); err != nil {
		t.Fatal(err)
	}

	objects, err := vault.Open(t.TempDir(), projects)
	if err != nil {
		t.Fatal(err)
	}
	mediaA, err := objects.ImportManaged(ctx, vault.ImportParams{
		ProjectID: projectA.ID, Name: "ACL A media", OriginalFilename: "a.wav",
	}, strings.NewReader("project A private bytes"))
	if err != nil {
		t.Fatal(err)
	}
	mediaB, err := objects.ImportManaged(ctx, vault.ImportParams{
		ProjectID: projectB.ID, Name: "ACL B media", OriginalFilename: "b.wav",
	}, strings.NewReader("project B private bytes"))
	if err != nil {
		t.Fatal(err)
	}

	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	deviceRuntime := devicechannel.New(devices, nil)
	t.Cleanup(deviceRuntime.Close)
	companionAuth := companionauth.New(projects, time.Now)
	companionCredential, companionID := unifiedACLCompanionCredential(t, ctx, companionAuth)
	role, err := projects.CreateMachineRole(ctx, projectA.ID, store.CreateMachineRoleParams{
		RoleKey: "ACL-COMPANION", DisplayName: "ACL Companion",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.AssignMachineRole(ctx, role.ID, companionID); err != nil {
		t.Fatal(err)
	}
	companionRuntime := companionchannel.NewRuntime(projects, companionAuth)
	t.Cleanup(companionRuntime.Close)
	bulkManager := bulk.New(func(context.Context) (bulk.Mode, error) { return bulk.ModeEdit, nil })
	audit, err := securityaudit.New(h.db.DB, nil)
	if err != nil {
		t.Fatal(err)
	}
	capsules := &fakeShowCapsuleService{}
	audio := &fakeStageAudio{}
	audioScope := &StageAudioProjectScope{ProjectID: projectA.ID, Access: access, Vault: objects}
	runtime := runtimecontrol.New(projects, capability.NewRegistry())
	root := t.TempDir()

	handler := New(
		WithScopedOperatorProjects(h.auth, projects, access),
		WithOperatorCuePublish(h.auth, projects, publish.New(projects, capability.NewRegistry())),
		WithOperatorRuntime(h.auth, projects, runtime),
		WithScopedProjectFilesRead(h.auth, access),
		WithScopedProjectMediaDownload(h.auth, access, projects, objects),
		WithScopedOperatorShowCapsules(h.auth, capsules, root, access),
		WithScopedLocalCapsuleExports(h.auth, access, root),
		WithScopedOperatorStageDeviceReads(h.auth, devices, deviceRuntime, projects, access),
		WithOperatorTabletController(h.auth, devices, deviceRuntime, projects),
		WithScopedOperatorStageAudio(h.auth, projects, audio, nil, stageaudio.NewShowFence(), audioScope),
		WithCompanionAuth(companionAuth),
		WithCompanionRuntime(companionRuntime),
		WithVault(objects),
		WithCompanionVaultScope(projects),
		WithCompanionCaptureUpload(companionAuth, projects, objects, bulkManager),
		WithOperatorProjectAccess(h.auth, access, projects, audit),
		WithOwnerProjectCutoverPreview(h.auth, access),
		WithOwnerCapsuleImportReview(h.auth, capsules, root),
		WithOwnerPluginAdministration(h.auth),
		WithProjectRouteQualification(access),
		WithBulkManager(bulkManager),
	).Handler()

	request := func(login userauth.Credential, method, path, body string, want int) string {
		t.Helper()
		var req *http.Request
		if method == http.MethodGet {
			req = authenticatedReadRequest(method, path, login.Token)
		} else {
			req = authenticatedMutationRequest(t, method, path, []byte(body), login.Token, login.CSRFToken)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res.Body.String()
	}

	for _, login := range []userauth.Credential{operatorLogin, viewerLogin} {
		listing := request(login, http.MethodGet, "/api/v1/projects", "", http.StatusOK)
		if !strings.Contains(listing, projectA.ID) || strings.Contains(listing, projectB.ID) {
			t.Fatalf("project listing crossed its grant boundary: %s", listing)
		}
		request(login, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/runtime", "", http.StatusOK)
		request(login, http.MethodGet, "/api/v1/projects/"+projectB.ID+"/runtime", "", http.StatusNotFound)
		request(login, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/files", "", http.StatusOK)
		request(login, http.MethodGet, "/api/v1/projects/"+projectB.ID+"/files", "", http.StatusNotFound)
		request(login, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/files/"+mediaA.Version.ID+"/download", "", http.StatusOK)
		request(login, http.MethodGet, "/api/v1/projects/"+projectB.ID+"/files/"+mediaA.Version.ID+"/download", "", http.StatusNotFound)
		request(login, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/stage-devices", "", http.StatusOK)
		request(login, http.MethodGet, "/api/v1/projects/"+projectB.ID+"/stage-devices", "", http.StatusNotFound)
	}
	ownerListing := request(owner, http.MethodGet, "/api/v1/projects", "", http.StatusOK)
	if !strings.Contains(ownerListing, projectA.ID) || !strings.Contains(ownerListing, projectB.ID) {
		t.Fatalf("Owner did not retain cross-project administration in scoped assembly: %s", ownerListing)
	}
	request(owner, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/access/users/"+operator.ID, "", http.StatusOK)
	request(operatorLogin, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/access/users/"+operator.ID, "", http.StatusForbidden)
	cutoverPreview := request(owner, http.MethodGet, "/api/v1/project-access/cutover-preview", "", http.StatusOK)
	if !strings.Contains(cutoverPreview, "\"read_only\":true") || !strings.Contains(cutoverPreview, "\"enforced\":false") ||
		strings.Contains(cutoverPreview, "\"ready_for_cutover\":true") {
		t.Fatalf("production-shaped cutover preview did not retain the disabled/read-only state: %s", cutoverPreview)
	}
	request(operatorLogin, http.MethodGet, "/api/v1/project-access/cutover-preview", "", http.StatusForbidden)
	request(owner, http.MethodGet, "/api/v1/show-capsules/imports/review", "", http.StatusOK)
	request(operatorLogin, http.MethodGet, "/api/v1/show-capsules/imports/review", "", http.StatusForbidden)
	const cueBody = `{"display_label":"1","name":"Unified cue","order_index":1,"cue_type":"STANDARD","criticality":"NORMAL","enabled":true,"execution_policy":{},"actions":[]}`
	request(operatorLogin, http.MethodPost, "/api/v1/projects/"+projectA.ID+"/cues", cueBody, http.StatusCreated)
	request(operatorLogin, http.MethodPost, "/api/v1/projects/"+projectB.ID+"/cues", cueBody, http.StatusNotFound)
	request(viewerLogin, http.MethodPost, "/api/v1/projects/"+projectA.ID+"/cues", cueBody, http.StatusNotFound)
	request(operatorLogin, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/show-capsules/exports", "", http.StatusOK)
	request(operatorLogin, http.MethodGet, "/api/v1/projects/"+projectB.ID+"/show-capsules/exports", "", http.StatusNotFound)
	request(viewerLogin, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/show-capsules/exports", "", http.StatusNotFound)
	request(operatorLogin, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/show-capsule/preview", "", http.StatusOK)
	request(operatorLogin, http.MethodPost, "/api/v1/projects/"+projectA.ID+"/tablet-controller/commands", `{"command_type":"TABLET_PLAY","session_id":"not-active","devices":[]}`, http.StatusForbidden)

	request(operatorLogin, http.MethodGet, "/api/v1/system/audio/status?project_id="+projectA.ID, "", http.StatusOK)
	request(operatorLogin, http.MethodGet, "/api/v1/system/audio/status?project_id="+projectB.ID, "", http.StatusNotFound)
	request(operatorLogin, http.MethodPost, "/api/v1/system/audio/command?project_id="+projectA.ID,
		`{"operation":"STOP","deck":"A"}`, http.StatusOK)
	request(operatorLogin, http.MethodPost, "/api/v1/system/audio/command?project_id="+projectB.ID,
		`{"operation":"STOP","deck":"A"}`, http.StatusNotFound)
	if len(audio.calls) != 2 {
		t.Fatalf("StageAudio accepted %d operations, want only the Project A status and STOP", len(audio.calls))
	}

	// The browser's global role and project grants never authenticate Companion
	// raw-hash or runtime routes; those require a device runtime session.
	for _, path := range []string{
		"/api/v1/companion/runtime",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "127.0.0.1:41000"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("browser session reached Companion endpoint %s: %d %s", path, res.Code, res.Body.String())
		}
	}
	for _, test := range []struct {
		contentHash string
		want        int
	}{
		{contentHash: mediaA.Version.ContentHash, want: http.StatusOK},
		{contentHash: mediaB.Version.ContentHash, want: http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vault/objects/"+test.contentHash, nil)
		req.RemoteAddr = "127.0.0.1:41000"
		req.Header.Set("Authorization", "StageCoreSession "+companionCredential.Token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != test.want {
			t.Fatalf("Companion hash %s returned %d, want %d: %s", test.contentHash, res.Code, test.want, res.Body.String())
		}
	}
	// A valid Companion session still needs a browser-issued capture ticket;
	// its device role alone cannot create or complete arbitrary uploads.
	upload := httptest.NewRequest(http.MethodPut, "/api/v1/companion/capture-uploads/not-a-ticket", strings.NewReader("payload"))
	upload.RemoteAddr = "127.0.0.1:41000"
	upload.Header.Set("Authorization", "StageCoreSession "+companionCredential.Token)
	upload.ContentLength = int64(len("payload"))
	uploadResponse := httptest.NewRecorder()
	handler.ServeHTTP(uploadResponse, upload)
	if uploadResponse.Code != http.StatusUnauthorized || !strings.Contains(uploadResponse.Body.String(), "CAPTURE_UPLOAD") {
		t.Fatalf("unticketed Companion upload returned %d: %s", uploadResponse.Code, uploadResponse.Body.String())
	}
}

func unifiedACLCompanionCredential(t *testing.T, ctx context.Context, auth *companionauth.Service) (companionauth.RuntimeSessionCredential, string) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := elliptic.Marshal(elliptic.P256(), privateKey.X, privateKey.Y)
	clientNonce := make([]byte, 32)
	if _, err := rand.Read(clientNonce); err != nil {
		t.Fatal(err)
	}
	const companionID = "55555555-5555-4555-8555-555555555599"
	receipt, err := auth.RequestPairing(ctx, companionauth.PairingRequestInput{
		CompanionID: companionID, DisplayName: "Unified ACL Companion", Hostname: "unified-acl.local",
		Platform: "macos", Architecture: "arm64", Version: "test", Capabilities: []string{"vault.read"},
		PublicKeyAlgorithm: domain.CompanionPublicKeyAlgorithm,
		PublicKeyBase64:    base64.StdEncoding.EncodeToString(publicKey),
		ClientNonceBase64:  base64.StdEncoding.EncodeToString(clientNonce),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.ApprovePairing(ctx, receipt.RequestID, receipt.PairingCode, companionauth.Approval{Actor: "owner", Authorized: true}); err != nil {
		t.Fatal(err)
	}
	challenge, err := auth.BeginAuthentication(ctx, companionID)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(companionauth.AuthenticationMessage(companionID, challenge.ChallengeID, challenge.NonceBase64))
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	credential, err := auth.CompleteAuthentication(ctx, companionID, challenge.ChallengeID, base64.StdEncoding.EncodeToString(signature))
	if err != nil {
		t.Fatal(err)
	}
	return credential, companionID
}
