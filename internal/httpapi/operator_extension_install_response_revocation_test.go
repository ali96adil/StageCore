package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

type extensionSetRevocationFixture struct {
	app        *authHarness
	store      *store.Store
	repository *software.Repository
	library    *extension.Library
	installer  *extension.Installer
	audit      *securityaudit.Service
	handler    http.Handler
	owner      userauth.Credential
	packageID  string
	set        extension.ExtensionSetManifest
}

func newExtensionSetRevocationFixture(t *testing.T) *extensionSetRevocationFixture {
	t.Helper()
	app := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(app.db.DB, clock.Real{})
	objects, err := vault.Open(t.TempDir(), stageStore)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := software.New(objects, stageStore, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	const extensionID = "test.restore-addon"
	softwarePackage, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: extensionID, Version: "1.0.0", Platform: runtime.GOOS, Architecture: runtime.GOARCH,
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "restore-addon",
		SigningStatus: store.SoftwareSigningUnknown, NotarizationStatus: store.SoftwareNotarizationUnknown,
		ReleaseChannel: store.SoftwareChannelDevelopment,
	}, strings.NewReader("extension set restore payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest := extension.Manifest{
		SchemaVersion: extension.ManifestSchemaVersion,
		ExtensionID:   extensionID,
		Version:       "1.0.0",
		Kind:          extension.KindPlugin,
		Source:        extension.SourceLocal,
		Permissions:   []string{"network.udp.send"},
		Name:          extension.LocalizedText{EN: "Restore Addon", ArIQ: "إضافة استعادة"},
		Summary:       extension.LocalizedText{EN: "Qualification addon", ArIQ: "إضافة تأهيل"},
		Compatibility: extension.Compatibility{
			APIMin: 1, APIMax: 1,
			Platforms: []string{runtime.GOOS}, Architectures: []string{runtime.GOARCH},
		},
	}
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := library.Register(ctx, softwarePackage.ID, rawManifest, "owner")
	if err != nil {
		t.Fatal(err)
	}
	installer, err := extension.NewInstaller(library, filepath.Join(t.TempDir(), "extensions"))
	if err != nil {
		t.Fatal(err)
	}
	audit, err := securityaudit.New(app.db.DB, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorExtensionInstaller(app.auth, installer, audit)).Handler()
	owner, err := app.auth.Login(ctx, "owner", app.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	set := extension.ExtensionSetManifest{
		Format: extension.ExtensionSetFormatV1, SchemaVersion: extension.ExtensionSetSchemaVersion,
		Extensions: []extension.ExtensionSetEntry{{
			ExtensionID:      registered.Manifest.ExtensionID,
			Version:          registered.Manifest.Version,
			Kind:             registered.Manifest.Kind,
			Source:           registered.Manifest.Source,
			ManifestSHA256:   registered.ManifestSHA256,
			PayloadSHA256:    softwarePackage.ContentHash,
			PayloadSizeBytes: softwarePackage.SizeBytes,
			Platform:         softwarePackage.Platform,
			Architecture:     softwarePackage.Architecture,
		}},
	}
	return &extensionSetRevocationFixture{
		app: app, store: stageStore, repository: repository, library: library, installer: installer,
		audit: audit, handler: handler, owner: owner, packageID: softwarePackage.ID, set: set,
	}
}

func TestExtensionPermissionDecisionSuppressesCommittedResultAfterRevocation(t *testing.T) {
	fixture := newExtensionSetRevocationFixture(t)
	installed, err := fixture.installer.InstallPlanned(context.Background(), fixture.packageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := extension.NewPermissionReviewer(fixture.installer)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorExtensionPermissionReview(fixture.app.auth, reviewer, fixture.audit)).Handler()
	revokeExtensionSessionAfterAudit(t, fixture, "extension.permission.review")
	request := authenticatedMutationRequest(t, http.MethodPut,
		"/api/v1/extensions/installations/"+installed.InstallationID+"/permissions/network.udp.send",
		[]byte(`{"decision":"APPROVED"}`), fixture.owner.Token, fixture.owner.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"effect_may_have_been_committed":true`) ||
		!strings.Contains(response.Body.String(), `"retryable":false`) || strings.Contains(response.Body.String(), `"status":"APPROVED"`) {
		t.Fatalf("revoked permission decision leaked its result or allowed replay: status=%d body=%s", response.Code, response.Body.String())
	}
	review, err := reviewer.Get(context.Background(), installed.InstallationID)
	if err != nil || len(review.Items) != 1 || review.Items[0].Decision != extension.PermissionDecisionApproved {
		t.Fatalf("permission decision should remain committed after response revocation: review=%+v err=%v", review, err)
	}
}

func revokeExtensionSessionAfterAudit(t *testing.T, fixture *extensionSetRevocationFixture, eventType string) {
	t.Helper()
	_, err := fixture.app.db.DB.ExecContext(context.Background(), `
		CREATE TRIGGER revoke_extension_session_after_audit
		AFTER INSERT ON security_audit_records
		WHEN NEW.event_type = '`+eventType+`' AND NEW.result = 'SUCCESS'
		BEGIN
			UPDATE browser_sessions
			SET revoked_at_us = NEW.occurred_at_us
			WHERE user_id = NEW.actor_user_id AND revoked_at_us IS NULL;
		END`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExtensionPackageInstallSuppressesCommittedResultAfterRevocation(t *testing.T) {
	fixture := newExtensionSetRevocationFixture(t)
	revokeExtensionSessionAfterAudit(t, fixture, "extension.package.install")
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/extensions/packages/"+fixture.packageID+"/install", nil,
		fixture.owner.Token, fixture.owner.CSRFToken)
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"effect_may_have_been_committed":true`) ||
		!strings.Contains(response.Body.String(), `"retryable":false`) || strings.Contains(response.Body.String(), "installation_id") {
		t.Fatalf("revoked install response leaked its result or allowed replay: status=%d body=%s", response.Code, response.Body.String())
	}
	items, err := fixture.installer.List(context.Background(), "")
	if err != nil || len(items) != 1 {
		t.Fatalf("install should remain committed after response revocation: items=%+v err=%v", items, err)
	}
}

func TestExtensionSetRestoreSuppressesCommittedResultAfterRevocation(t *testing.T) {
	fixture := newExtensionSetRevocationFixture(t)
	revokeExtensionSessionAfterAudit(t, fixture, "extension.set_manifest.restore")
	raw, err := json.Marshal(fixture.set)
	if err != nil {
		t.Fatal(err)
	}
	request := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/extensions/set-manifest/restore", raw,
		fixture.owner.Token, fixture.owner.CSRFToken)
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"effect_may_have_been_committed":true`) ||
		!strings.Contains(response.Body.String(), `"retryable":false`) || strings.Contains(response.Body.String(), "installed") ||
		strings.Contains(response.Body.String(), "installation_id") {
		t.Fatalf("revoked restore response leaked its result or allowed replay: status=%d body=%s", response.Code, response.Body.String())
	}
	items, err := fixture.installer.List(context.Background(), "")
	if err != nil || len(items) != 1 {
		t.Fatalf("restore should remain committed after response revocation: items=%+v err=%v", items, err)
	}
}

func TestExtensionSetRestorePlanDoesNotReturnAfterSessionRevocation(t *testing.T) {
	fixture := newExtensionSetRevocationFixture(t)
	raw, err := json.Marshal(extension.ExtensionSetManifest{
		Format: extension.ExtensionSetFormatV1, SchemaVersion: extension.ExtensionSetSchemaVersion,
		Extensions: []extension.ExtensionSetEntry{},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/extensions/set-manifest/restore-plan", raw,
		fixture.owner.Token, fixture.owner.CSRFToken)
	request.Body = &firmwareUploadRevokeReader{ReadCloser: request.Body, revoke: func() {
		if err := fixture.app.auth.Logout(context.Background(), fixture.owner.Token); err != nil {
			t.Fatal(err)
		}
	}}
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), `"status":"NOOP"`) {
		t.Fatalf("revoked session received restore plan: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestExtensionSetExportDoesNotReturnAfterSessionRevocation(t *testing.T) {
	fixture := newExtensionSetRevocationFixture(t)
	revokeExtensionSessionAfterAudit(t, fixture, "extension.set_manifest.export")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/set-manifest", nil)
	request.RemoteAddr = "127.0.0.1:19604"
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: fixture.owner.Token})
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), extension.ExtensionSetFormatV1) ||
		response.Header().Get("Content-Disposition") != "" {
		t.Fatalf("revoked session received extension set export: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}
