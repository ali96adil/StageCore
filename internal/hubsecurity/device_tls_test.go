package hubsecurity

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestDeviceTLSCertificateIsStableAndBoundToHubIdentity(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	service, err := Open(ctx, db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := service.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}

	first, firstPin, err := service.DeviceTLSCertificate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, secondPin, err := service.DeviceTLSCertificate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if firstPin == "" || firstPin != secondPin {
		t.Fatalf("device certificate pin changed: %q != %q", firstPin, secondPin)
	}
	if len(first.Certificate) != 1 || len(second.Certificate) != 1 {
		t.Fatalf("unexpected certificate chain lengths: %d, %d", len(first.Certificate), len(second.Certificate))
	}
	if string(first.Certificate[0]) != string(second.Certificate[0]) {
		t.Fatal("deterministic device certificate bytes changed between calls")
	}
	leaf := first.Leaf
	if leaf == nil {
		t.Fatal("device certificate has no parsed leaf")
	}
	if err := leaf.CheckSignatureFrom(leaf); err != nil {
		t.Fatalf("device certificate is not self-signed: %v", err)
	}
	public, ok := leaf.PublicKey.(ed25519.PublicKey)
	if !ok {
		t.Fatalf("leaf public key type = %T, want ed25519.PublicKey", leaf.PublicKey)
	}
	digest := sha256.Sum256(public)
	gotFingerprint := "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:])
	if gotFingerprint != identity.Fingerprint {
		t.Fatalf("TLS public identity fingerprint=%q, Hub fingerprint=%q", gotFingerprint, identity.Fingerprint)
	}
}
