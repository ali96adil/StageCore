package hubsecurity

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"time"
)

var (
	deviceCertificateNotBefore = time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	deviceCertificateNotAfter  = time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
)

// DeviceTLSCertificate returns the deterministic self-signed certificate used
// by the F-004 LAN device gateway. It deliberately reuses the durable Hub
// Ed25519 identity key: the Hub has one cryptographic identity, not a parallel
// discovery/TLS identity that could drift from pairing trust.
//
// The returned SHA-256 value is the lowercase hex digest of the leaf DER and
// is safe to advertise as a public certificate pin. It is not a secret.
func (s *Service) DeviceTLSCertificate(ctx context.Context) (tls.Certificate, string, error) {
	if s == nil {
		return tls.Certificate{}, "", fmt.Errorf("Hub security service is required")
	}
	identity, err := s.ensureIdentity(ctx)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	privateBytes, err := os.ReadFile(s.identityKeyPath())
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("read Hub identity key for device TLS: %w", err)
	}
	if len(privateBytes) != ed25519.PrivateKeySize {
		return tls.Certificate{}, "", fmt.Errorf("%w: invalid private key length", ErrIdentityMismatch)
	}
	private := ed25519.PrivateKey(privateBytes)
	public := private.Public().(ed25519.PublicKey)
	if fingerprint(public) != identity.Fingerprint {
		return tls.Certificate{}, "", ErrIdentityMismatch
	}

	idDigest := sha256.Sum256([]byte(identity.HubID))
	serial := new(big.Int).SetBytes(idDigest[:16])
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	keyDigest := sha256.Sum256(public)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"StageCore"},
			CommonName:   "StageCore Hub Device Gateway",
		},
		NotBefore:             deviceCertificateNotBefore,
		NotAfter:              deviceCertificateNotAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		SubjectKeyId:          append([]byte(nil), keyDigest[:20]...),
	}
	der, err := x509.CreateCertificate(nil, template, template, public, private)
	if err != nil {
		// crypto/x509 currently does not consume randomness for Ed25519
		// signatures. A nil reader keeps the resulting certificate explicitly
		// deterministic; if that contract changes, fail rather than silently
		// rotating the advertised pin at restart.
		return tls.Certificate{}, "", fmt.Errorf("create deterministic device TLS certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("parse device TLS certificate: %w", err)
	}
	certificateDigest := sha256.Sum256(der)
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  private,
		Leaf:        leaf,
	}, hex.EncodeToString(certificateDigest[:]), nil
}
