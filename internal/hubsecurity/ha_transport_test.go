package hubsecurity

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/hawitness"
)

func TestHATransportCertificateReusesDurableHubIdentity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	handle, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	service, err := Open(ctx, handle.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := service.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := service.HATransportCertificate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	certificateID, err := hawitness.CertificateIdentity(certificate.Leaf, hawitness.RoleHub)
	if err != nil {
		t.Fatal(err)
	}
	if certificateID != identity.HubID {
		t.Fatalf("HA certificate Hub ID=%q, want %q", certificateID, identity.HubID)
	}
	certificateFingerprint, err := hawitness.CertificateFingerprint(certificate.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	if certificateFingerprint != identity.Fingerprint {
		t.Fatalf("HA certificate fingerprint=%q, want %q", certificateFingerprint, identity.Fingerprint)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	service2, err := Open(ctx, reopened.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	certificate2, err := service2.HATransportCertificate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint2, err := hawitness.CertificateFingerprint(certificate2.Leaf)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint2 != identity.Fingerprint {
		t.Fatalf("reopened HA certificate fingerprint=%q, want %q", fingerprint2, identity.Fingerprint)
	}
}
