package hubsecurity

import (
	"context"
	"crypto/ed25519"
	"os"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
)

func TestOperatorTLSCertificateIsStableSeparateAndHasBonjourSAN(t *testing.T) {
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

	device, devicePin, err := service.DeviceTLSCertificate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, host, firstPin, err := service.OperatorTLSCertificate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, secondHost, secondPin, err := service.OperatorTLSCertificate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if host != operatorTLSHostName(identity.HubID) || secondHost != host {
		t.Fatalf("Operator host mismatch: %q %q", host, secondHost)
	}
	if firstPin == "" || firstPin != secondPin {
		t.Fatalf("Operator certificate pin changed: %q != %q", firstPin, secondPin)
	}
	if firstPin == devicePin || string(first.Certificate[0]) == string(device.Certificate[0]) {
		t.Fatal("Operator TLS certificate must be domain-separated from device TLS")
	}
	if first.Leaf == nil || len(first.Leaf.DNSNames) != 1 || first.Leaf.DNSNames[0] != host {
		t.Fatalf("Operator certificate SANs=%v want %q", first.Leaf.DNSNames, host)
	}
	if err := first.Leaf.VerifyHostname(host); err != nil {
		t.Fatalf("Operator certificate does not verify Bonjour hostname: %v", err)
	}
	if string(first.Certificate[0]) != string(second.Certificate[0]) {
		t.Fatal("Operator certificate bytes changed between calls")
	}

	privateBytes, err := os.ReadFile(service.identityKeyPath())
	if err != nil {
		t.Fatal(err)
	}
	identityPrivate := ed25519.PrivateKey(privateBytes)
	deviceKey, err := deriveDeviceTLSTransportKey(identityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	operatorKey, err := deriveOperatorTLSTransportKey(identityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if deviceKey.D.Cmp(operatorKey.D) == 0 {
		t.Fatal("Operator and device TLS keys are not domain-separated")
	}
}
