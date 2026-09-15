package hawitness

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/halease"
)

func TestAuthenticatedWitnessTransportFencesHubsAndRejectsSpoofing(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	authority, err := halease.Open(ctx, halease.Config{
		Path: filepath.Join(t.TempDir(), "witness.sqlite3"),
		LeaseDuration: 5 * time.Second,
	}, clock.Fixed{Time: now})
	if err != nil {
		t.Fatalf("open lease authority: %v", err)
	}
	defer authority.Close()

	witness := testIdentity(t, "witness-test", RoleWitness, now)
	hubA := testIdentity(t, "hub-a", RoleHub, now)
	hubB := testIdentity(t, "hub-b", RoleHub, now)
	server, err := NewServer(authority, ServerConfig{
		AuthorizedHubs: map[string]string{
			hubA.ID: hubA.Fingerprint,
			hubB.ID: hubB.Fingerprint,
		},
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	tlsConfig, err := server.TLSConfig(witness)
	if err != nil {
		t.Fatalf("TLSConfig: %v", err)
	}
	httpServer := httptest.NewUnstartedServer(server.Handler())
	httpServer.TLS = tlsConfig
	httpServer.StartTLS()
	defer httpServer.Close()

	clientA := testClient(t, httpServer.URL, hubA, witness, now)
	clientB := testClient(t, httpServer.URL, hubB, witness, now)

	first, err := clientA.Acquire(ctx)
	if err != nil {
		t.Fatalf("hub A acquire: %v", err)
	}
	if first.HolderID != hubA.ID || first.Epoch != 1 || !first.Active {
		t.Fatalf("unexpected first lease: %+v", first.LeaseResponse)
	}
	deadline, err := first.ConservativeDeadline()
	if err != nil {
		t.Fatalf("conservative deadline: %v", err)
	}
	if want := now.Add(5 * time.Second); !deadline.Equal(want) {
		t.Fatalf("conservative deadline = %s, want %s", deadline, want)
	}

	if _, err := clientB.Acquire(ctx); !remoteErrorIs(err, http.StatusConflict, "LEASE_HELD") {
		t.Fatalf("hub B acquire while A holds lease = %v, want LEASE_HELD", err)
	}

	spoofRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, httpServer.URL+"/v1/lease/acquire", bytes.NewBufferString(`{"holder_id":"hub-b"}`))
	if err != nil {
		t.Fatalf("build spoof request: %v", err)
	}
	spoofRequest.Header.Set("Content-Type", "application/json")
	spoofResponse, err := clientA.http.Do(spoofRequest)
	if err != nil {
		t.Fatalf("spoof request transport: %v", err)
	}
	defer spoofResponse.Body.Close()
	if spoofResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("spoof request status = %d, want %d", spoofResponse.StatusCode, http.StatusBadRequest)
	}

	if _, err := clientA.Renew(ctx, first.Epoch+1); !remoteErrorIs(err, http.StatusConflict, "LEASE_NOT_HELD") {
		t.Fatalf("stale renewal = %v, want LEASE_NOT_HELD", err)
	}
	renewed, err := clientA.Renew(ctx, first.Epoch)
	if err != nil {
		t.Fatalf("hub A renew: %v", err)
	}
	if renewed.Epoch != first.Epoch || renewed.HolderID != hubA.ID {
		t.Fatalf("unexpected renewed lease: %+v", renewed.LeaseResponse)
	}
	if err := clientA.Release(ctx, first.Epoch); err != nil {
		t.Fatalf("hub A release: %v", err)
	}

	second, err := clientB.Acquire(ctx)
	if err != nil {
		t.Fatalf("hub B acquire after release: %v", err)
	}
	if second.HolderID != hubB.ID || second.Epoch != 2 || !second.Active {
		t.Fatalf("unexpected second lease: %+v", second.LeaseResponse)
	}
	current, err := clientA.Current(ctx)
	if err != nil {
		t.Fatalf("current lease: %v", err)
	}
	if current.HolderID != hubB.ID || current.Epoch != second.Epoch || !current.Active {
		t.Fatalf("unexpected current lease: %+v", current.LeaseResponse)
	}
}

func TestWitnessTransportRejectsUnauthorizedHubAndWrongWitnessPin(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	authority, err := halease.Open(ctx, halease.Config{
		Path: filepath.Join(t.TempDir(), "witness.sqlite3"),
		LeaseDuration: 5 * time.Second,
	}, clock.Fixed{Time: now})
	if err != nil {
		t.Fatalf("open lease authority: %v", err)
	}
	defer authority.Close()

	witness := testIdentity(t, "witness-test", RoleWitness, now)
	authorized := testIdentity(t, "hub-authorized", RoleHub, now)
	unauthorized := testIdentity(t, "hub-unauthorized", RoleHub, now)
	server, err := NewServer(authority, ServerConfig{
		AuthorizedHubs: map[string]string{authorized.ID: authorized.Fingerprint},
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	tlsConfig, err := server.TLSConfig(witness)
	if err != nil {
		t.Fatalf("TLSConfig: %v", err)
	}
	httpServer := httptest.NewUnstartedServer(server.Handler())
	httpServer.TLS = tlsConfig
	httpServer.StartTLS()
	defer httpServer.Close()

	unauthorizedClient := testClient(t, httpServer.URL, unauthorized, witness, now)
	if _, err := unauthorizedClient.Acquire(ctx); err == nil {
		t.Fatal("expected unauthorized Hub TLS authentication to fail")
	}

	wrongWitness := testIdentity(t, "other-witness", RoleWitness, now)
	wrongPinClient, err := NewClient(ClientConfig{
		BaseURL: httpServer.URL,
		HubCertificate: authorized.Certificate,
		WitnessID: witness.ID,
		WitnessFingerprint: wrongWitness.Fingerprint,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewClient wrong pin: %v", err)
	}
	if _, err := wrongPinClient.Acquire(ctx); err == nil {
		t.Fatal("expected wrong witness fingerprint to fail closed")
	}
}

func testIdentity(t *testing.T, id, role string, now time.Time) TransportIdentity {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate %s identity: %v", role, err)
	}
	certificate, err := NewCertificate(id, role, privateKey, now)
	if err != nil {
		t.Fatalf("NewCertificate %s: %v", role, err)
	}
	fingerprint, err := CertificateFingerprint(certificate.Leaf)
	if err != nil {
		t.Fatalf("CertificateFingerprint %s: %v", role, err)
	}
	return TransportIdentity{ID: id, Fingerprint: fingerprint, Certificate: certificate}
}

func testClient(t *testing.T, baseURL string, hub, witness TransportIdentity, now time.Time) *Client {
	t.Helper()
	client, err := NewClient(ClientConfig{
		BaseURL: baseURL,
		HubCertificate: hub.Certificate,
		WitnessID: witness.ID,
		WitnessFingerprint: witness.Fingerprint,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func remoteErrorIs(err error, status int, code string) bool {
	var remote *RemoteError
	return errors.As(err, &remote) && remote.StatusCode == status && remote.Code == code
}
