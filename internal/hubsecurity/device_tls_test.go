package hubsecurity

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"os"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
)

func TestDeviceTLSCertificateIsStableAndBoundToHubIdentity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	handle, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	service, err := Open(ctx, handle.DB, root)
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
	if leaf.SignatureAlgorithm != x509.PureEd25519 {
		t.Fatalf("leaf signature algorithm = %v, want Ed25519", leaf.SignatureAlgorithm)
	}
	if leaf.IsCA {
		t.Fatal("device certificate must remain an end-entity certificate, not a CA")
	}

	transportPublic, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("leaf public key type = %T, want *ecdsa.PublicKey", leaf.PublicKey)
	}
	if transportPublic.Curve.Params().Name != elliptic.P256().Params().Name {
		t.Fatalf("leaf curve = %v, want P-256", transportPublic.Curve.Params().Name)
	}
	transportPrivate, ok := first.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("TLS private key type = %T, want *ecdsa.PrivateKey", first.PrivateKey)
	}
	if transportPrivate.Curve.Params().Name != elliptic.P256().Params().Name ||
		transportPrivate.PublicKey.X.Cmp(transportPublic.X) != 0 ||
		transportPrivate.PublicKey.Y.Cmp(transportPublic.Y) != 0 {
		t.Fatal("TLS private key does not match the P-256 leaf public key")
	}

	identityPrivateBytes, err := os.ReadFile(service.identityKeyPath())
	if err != nil {
		t.Fatal(err)
	}
	identityPrivate := ed25519.PrivateKey(identityPrivateBytes)
	identityPublic := identityPrivate.Public().(ed25519.PublicKey)
	if !ed25519.Verify(identityPublic, leaf.RawTBSCertificate, leaf.Signature) {
		t.Fatal("device certificate is not signed by the durable Hub Ed25519 identity")
	}
	digest := sha256.Sum256(identityPublic)
	gotFingerprint := "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:])
	if gotFingerprint != identity.Fingerprint {
		t.Fatalf("Hub identity fingerprint=%q, stored fingerprint=%q", gotFingerprint, identity.Fingerprint)
	}
}

func TestDeriveDeviceTLSTransportKeyIsDeterministicAndDomainSeparated(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	identityPrivate := ed25519.NewKeyFromSeed(seed)

	first, err := deriveDeviceTLSTransportKey(identityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	second, err := deriveDeviceTLSTransportKey(identityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if first.Curve.Params().Name != elliptic.P256().Params().Name || second.Curve.Params().Name != elliptic.P256().Params().Name {
		t.Fatal("derived device TLS key must use P-256")
	}
	if first.D.Cmp(second.D) != 0 ||
		first.PublicKey.X.Cmp(second.PublicKey.X) != 0 ||
		first.PublicKey.Y.Cmp(second.PublicKey.Y) != 0 {
		t.Fatal("derived device TLS key changed for the same Hub identity")
	}
	if first.D.Sign() <= 0 || first.D.Cmp(elliptic.P256().Params().N) >= 0 {
		t.Fatal("derived device TLS scalar is outside the valid P-256 range")
	}
}
