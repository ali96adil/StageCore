package companionauth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestPairApproveAuthenticateAndRevokeCompanion(t *testing.T) {
	ctx := context.Background()
	service, s, now := newFixture(t)
	privateKey, publicKey := deviceKey(t)
	companionID := "11111111-1111-4111-8111-111111111111"

	receipt, err := service.RequestPairing(ctx, pairingInput(companionID, publicKey))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.PairingCode == "" || !receipt.ExpiresAt.After(now.Now()) {
		t.Fatalf("receipt=%#v", receipt)
	}

	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Pairing Gate"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{RoleKey: "VIDEO-MAIN", Required: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignMachineRole(ctx, role.ID, companionID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("unpaired Companion reached runtime assignment: %v", err)
	}
	if _, err := service.BeginAuthentication(ctx, companionID); companionauth.ErrorCode(err) != companionauth.CodeUnpaired {
		t.Fatalf("unpaired authentication error=%v", err)
	}
	if _, err := service.ApprovePairing(ctx, receipt.RequestID, receipt.PairingCode, companionauth.Approval{Actor: "owner", Authorized: false}); companionauth.ErrorCode(err) != companionauth.CodeApprovalUnauthorized {
		t.Fatalf("unauthorized approval error=%v", err)
	}

	key, err := service.ApprovePairing(ctx, receipt.RequestID, receipt.PairingCode, companionauth.Approval{Actor: "owner", Authorized: true})
	if err != nil {
		t.Fatal(err)
	}
	if key.CompanionID != companionID || key.PublicKeyFingerprint == "" {
		t.Fatalf("trusted key=%#v", key)
	}
	companion, err := s.GetCompanion(ctx, companionID)
	if err != nil || companion.TrustState != domain.CompanionTrusted {
		t.Fatalf("paired Companion=%#v err=%v", companion, err)
	}

	challenge, err := service.BeginAuthentication(ctx, companionID)
	if err != nil {
		t.Fatal(err)
	}
	wrongPrivateKey, _ := deviceKey(t)
	wrongSignature := sign(t, wrongPrivateKey, companionauth.AuthenticationMessage(companionID, challenge.ChallengeID, challenge.NonceBase64))
	if _, err := service.CompleteAuthentication(ctx, companionID, challenge.ChallengeID, wrongSignature); companionauth.ErrorCode(err) != companionauth.CodeProofInvalid {
		t.Fatalf("companion_id-only authentication error=%v", err)
	}

	signature := sign(t, privateKey, companionauth.AuthenticationMessage(companionID, challenge.ChallengeID, challenge.NonceBase64))
	credential, err := service.CompleteAuthentication(ctx, companionID, challenge.ChallengeID, signature)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Token == "" || credential.SessionID == "" || !credential.ExpiresAt.After(now.Now()) {
		t.Fatalf("credential=%#v", credential)
	}
	validated, err := service.ValidateRuntimeSession(ctx, credential.Token)
	if err != nil || validated.CompanionID != companionID {
		t.Fatalf("validated=%#v err=%v", validated, err)
	}

	if err := service.Revoke(ctx, companionID, "owner", "replacement device", true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.BeginAuthentication(ctx, companionID); companionauth.ErrorCode(err) != companionauth.CodeRevoked {
		t.Fatalf("revoked reconnect error=%v", err)
	}
	if _, err := service.ValidateRuntimeSession(ctx, credential.Token); companionauth.ErrorCode(err) != companionauth.CodeSessionInvalid && companionauth.ErrorCode(err) != companionauth.CodeRevoked {
		t.Fatalf("revoked runtime session error=%v", err)
	}
}

func TestExpiredPairingRequestCannotBeApproved(t *testing.T) {
	ctx := context.Background()
	service, _, now := newFixture(t)
	_, publicKey := deviceKey(t)
	receipt, err := service.RequestPairing(ctx, pairingInput("22222222-2222-4222-8222-222222222222", publicKey))
	if err != nil {
		t.Fatal(err)
	}
	now.current = now.current.Add(6 * time.Minute)
	if _, err := service.ApprovePairing(ctx, receipt.RequestID, receipt.PairingCode, companionauth.Approval{Actor: "technician", Authorized: true}); companionauth.ErrorCode(err) != companionauth.CodePairingExpired {
		t.Fatalf("expired approval error=%v", err)
	}
	status, err := service.PairingStatus(ctx, receipt.RequestID, receipt.PairingCode)
	if err != nil || status != domain.CompanionPairingExpired {
		t.Fatalf("status=%s err=%v", status, err)
	}
}

func newFixture(t *testing.T) (*companionauth.Service, *store.Store, *mutableClock) {
	t.Helper()
	ctx := context.Background()
	now := &mutableClock{current: time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC)}
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	s := store.New(handle.DB, now)
	return companionauth.New(s, now.Now), s, now
}

type mutableClock struct{ current time.Time }

func (m *mutableClock) Now() time.Time { return m.current }

var _ clock.Clock = (*mutableClock)(nil)

func pairingInput(companionID, publicKey string) companionauth.PairingRequestInput {
	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)
	return companionauth.PairingRequestInput{
		CompanionID: companionID, DisplayName: "Video Mac", Hostname: "video.local",
		Platform: "macos", Architecture: "arm64", Version: "0.1.0", Capabilities: []string{"local.echo"},
		PublicKeyAlgorithm: domain.CompanionPublicKeyAlgorithm, PublicKeyBase64: publicKey,
		ClientNonceBase64: base64.StdEncoding.EncodeToString(nonce),
	}
}

func deviceKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicBytes := elliptic.Marshal(elliptic.P256(), privateKey.X, privateKey.Y)
	return privateKey, base64.StdEncoding.EncodeToString(publicBytes)
}

func sign(t *testing.T, privateKey *ecdsa.PrivateKey, message []byte) string {
	t.Helper()
	digest := sha256.Sum256(message)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}
