package httpapi_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestVaultObjectDownloadRequiresSessionAndSupportsRange(t *testing.T) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	s := store.New(handle.DB, clock.Real{})
	auth := companionauth.New(s, nil)
	credential := pairedRuntimeCredential(t, ctx, auth)

	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Transfer Test"})
	if err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(t.TempDir(), s)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("0123456789abcdefghijklmnopqrstuvwxyz"), 4000)
	managed, err := v.ImportManaged(ctx, vault.ImportParams{ProjectID: project.ID, Name: "Media", OriginalFilename: "media.bin"}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}

	api := httpapi.New(httpapi.WithCompanionAuth(auth), httpapi.WithVault(v))
	path := "/api/v1/vault/objects/" + managed.Object.ContentHash

	unauthorized := httptest.NewRequest(http.MethodGet, path, nil)
	unauthorized.RemoteAddr = "127.0.0.1:41000"
	unauthorizedResult := httptest.NewRecorder()
	api.Handler().ServeHTTP(unauthorizedResult, unauthorized)
	if unauthorizedResult.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d want %d", unauthorizedResult.Code, http.StatusUnauthorized)
	}

	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = "127.0.0.1:41001"
	request.Header.Set("Authorization", "StageCoreSession "+credential.Token)
	request.Header.Set("Range", "bytes=10-19")
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent {
		t.Fatalf("range status=%d body=%s", response.Code, response.Body.String())
	}
	if got, want := response.Body.Bytes(), payload[10:20]; !bytes.Equal(got, want) {
		t.Fatalf("range bytes=%q want=%q", got, want)
	}
	if got := response.Header().Get("Content-Range"); got != "bytes 10-19/144000" {
		t.Fatalf("Content-Range=%q", got)
	}
	if got := response.Header().Get("X-Content-SHA256"); got != managed.Object.ContentHash {
		t.Fatalf("X-Content-SHA256=%q want=%q", got, managed.Object.ContentHash)
	}
	if got := response.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges=%q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control=%q", got)
	}
	session, err := auth.ValidateRuntimeSession(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	stream := &releaseVaultResponse{
		ResponseRecorder: httptest.NewRecorder(),
		release: func() {
			if err := auth.Revoke(ctx, session.CompanionID, "owner", "test revocation", true); err != nil {
				t.Fatal(err)
			}
		},
	}
	request.Header.Del("Range")
	api.Handler().ServeHTTP(stream, request)
	if got := stream.Body.Len(); got == 0 || got > 64<<10 || got >= len(payload) {
		t.Fatalf("revoked session wrote %d bytes", got)
	}
	retry := httptest.NewRecorder()
	request.Header.Set("Range", "bytes=65536-")
	api.Handler().ServeHTTP(retry, request)
	if retry.Code != http.StatusGone {
		t.Fatalf("revoked range request returned HTTP %d", retry.Code)
	}
}

func TestScopedCompanionVaultDeniesCrossProjectAndStopsAfterRelease(t *testing.T) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	s := store.New(handle.DB, clock.Real{})
	auth := companionauth.New(s, nil)
	credential := pairedRuntimeCredential(t, ctx, auth)
	session, err := auth.ValidateRuntimeSession(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Assigned project"})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Other project"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "MAC-MEDIA", DisplayName: "Media Mac", RequiredCapabilities: []string{"local.echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(t.TempDir(), s)
	if err != nil {
		t.Fatal(err)
	}
	importMedia := func(projectID string, payload []byte) store.ManagedMedia {
		t.Helper()
		media, err := v.ImportManaged(ctx, vault.ImportParams{ProjectID: projectID, Name: "Media", OriginalFilename: "media.bin"}, bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		return media
	}
	payload := bytes.Repeat([]byte("abcd"), 1<<16)
	managed := importMedia(project.ID, payload)
	foreign := importMedia(other.ID, []byte("foreign"))
	requirement := func(media store.ManagedMedia) snapshot.RequiredMedia {
		return snapshot.RequiredMedia{
			MachineRoleID: role.ID, RoleKey: role.RoleKey, MediaAssetID: media.Asset.ID, ContentVersionID: media.Version.ID,
			ChecksumAlgorithm: "sha256", ContentHash: media.Object.ContentHash, SizeBytes: media.Object.SizeBytes, Required: true,
		}
	}
	manifest, err := json.Marshal(snapshot.Manifest{
		SchemaVersion: 4, ProjectID: project.ID, RevisionID: revision.ID,
		RequiredMedia: []snapshot.RequiredMedia{requirement(managed), requirement(foreign)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(manifest)
	if _, err := s.CreateRuntimeSnapshot(ctx, revision.ID, "test", hex.EncodeToString(digest[:]), manifest); err != nil {
		t.Fatal(err)
	}
	api := httpapi.New(httpapi.WithCompanionAuth(auth), httpapi.WithVault(v), httpapi.WithCompanionVaultScope(s))
	request := func(hash, byteRange string, writer http.ResponseWriter) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vault/objects/"+hash, nil)
		req.RemoteAddr = "127.0.0.1:41001"
		req.Header.Set("Authorization", "StageCoreSession "+credential.Token)
		req.Header.Set("Range", byteRange)
		api.Handler().ServeHTTP(writer, req)
	}
	assertDenied := func(hash string) {
		t.Helper()
		response := httptest.NewRecorder()
		request(hash, "", response)
		if response.Code != http.StatusNotFound {
			t.Fatalf("denied media returned %d", response.Code)
		}
	}
	assertDenied(managed.Object.ContentHash)
	assignment, err := s.AssignMachineRole(ctx, role.ID, session.CompanionID)
	if err != nil {
		t.Fatal(err)
	}
	assertDenied(foreign.Object.ContentHash)
	response := httptest.NewRecorder()
	request(managed.Object.ContentHash, "bytes=3-6", response)
	if response.Code != http.StatusPartialContent || !bytes.Equal(response.Body.Bytes(), payload[3:7]) {
		t.Fatalf("scoped range response=%d %q", response.Code, response.Body.Bytes())
	}
	stream := &releaseVaultResponse{
		ResponseRecorder: httptest.NewRecorder(),
		release: func() {
			if err := s.ReleaseRoleAssignment(ctx, assignment.ID); err != nil {
				t.Fatal(err)
			}
		},
	}
	request(managed.Object.ContentHash, "", stream)
	got := stream.Body.Bytes()
	if len(got) == 0 || len(got) > 64<<10 || !bytes.Equal(got, payload[:len(got)]) {
		t.Fatalf("released stream wrote %d bytes", len(got))
	}
	assertDenied(managed.Object.ContentHash)
}

type releaseVaultResponse struct {
	*httptest.ResponseRecorder
	release func()
}

func (w *releaseVaultResponse) Write(data []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(data)
	if w.release != nil {
		w.release()
		w.release = nil
	}
	return n, err
}

func pairedRuntimeCredential(t *testing.T, ctx context.Context, auth *companionauth.Service) companionauth.RuntimeSessionCredential {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicBytes := elliptic.Marshal(elliptic.P256(), privateKey.X, privateKey.Y)
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	companionID := "55555555-5555-4555-8555-555555555555"
	receipt, err := auth.RequestPairing(ctx, companionauth.PairingRequestInput{
		CompanionID: companionID, DisplayName: "Transfer Mac", Hostname: "transfer.local",
		Platform: "macos", Architecture: "arm64", Version: "0.1.0", Capabilities: []string{"local.echo"},
		PublicKeyAlgorithm: domain.CompanionPublicKeyAlgorithm,
		PublicKeyBase64:    base64.StdEncoding.EncodeToString(publicBytes),
		ClientNonceBase64:  base64.StdEncoding.EncodeToString(nonce),
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
	return credential
}
