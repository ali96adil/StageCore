package httpapi_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/httpapi"
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
	payload := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
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
	if got := response.Header().Get("Content-Range"); got != "bytes 10-19/36" {
		t.Fatalf("Content-Range=%q", got)
	}
	if got := response.Header().Get("X-Content-SHA256"); got != managed.Object.ContentHash {
		t.Fatalf("X-Content-SHA256=%q want=%q", got, managed.Object.ContentHash)
	}
	if got := response.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges=%q", got)
	}
}

func pairedRuntimeCredential(t *testing.T, ctx context.Context, auth *companionauth.Service) companionauth.RuntimeCredential {
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
		PublicKeyBase64: base64.StdEncoding.EncodeToString(publicBytes),
		ClientNonceBase64: base64.StdEncoding.EncodeToString(nonce),
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
