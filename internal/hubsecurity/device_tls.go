package hubsecurity

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
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

const deviceTLSTransportKeyContext = "StageCore Device TLS P-256 v1\x00"

// DeviceTLSCertificate returns the deterministic certificate used by the F-004
// LAN device gateway. The durable Hub Ed25519 identity remains authoritative:
// it signs the TLS leaf. The leaf itself uses a P-256 transport key because the
// Apple TLS stack used by the macOS Companion cannot negotiate a server
// CertificateVerify with the Hub's Ed25519 key.
//
// The P-256 transport key is deterministically and domain-separately derived
// from the durable Hub private key, so the certificate pin remains stable
// across restart, backup, and restore without introducing another persisted
// private key. Compromise of the transport key does not expose the Hub identity
// key through this one-way derivation.
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
	identityPrivate := ed25519.PrivateKey(privateBytes)
	identityPublic := identityPrivate.Public().(ed25519.PublicKey)
	if fingerprint(identityPublic) != identity.Fingerprint {
		return tls.Certificate{}, "", ErrIdentityMismatch
	}

	transportPrivate, err := deriveDeviceTLSTransportKey(identityPrivate)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	transportPublicDER, err := x509.MarshalPKIXPublicKey(&transportPrivate.PublicKey)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("marshal device TLS public key: %w", err)
	}

	idDigest := sha256.Sum256([]byte(identity.HubID))
	serial := new(big.Int).SetBytes(idDigest[:16])
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	identityKeyDigest := sha256.Sum256(identityPublic)
	transportKeyDigest := sha256.Sum256(transportPublicDER)

	issuer := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"StageCore"},
			CommonName:   "StageCore Hub Identity",
		},
		NotBefore:             deviceCertificateNotBefore,
		NotAfter:              deviceCertificateNotAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		SubjectKeyId:          append([]byte(nil), identityKeyDigest[:20]...),
		PublicKey:             identityPublic,
	}
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
		SubjectKeyId:          append([]byte(nil), transportKeyDigest[:20]...),
	}
	der, err := x509.CreateCertificate(nil, template, issuer, &transportPrivate.PublicKey, identityPrivate)
	if err != nil {
		// The certificate is signed by Ed25519, which is deterministic and does
		// not consume randomness. A nil reader keeps pin generation explicit: if
		// that contract changes, fail rather than silently rotating the pin.
		return tls.Certificate{}, "", fmt.Errorf("create deterministic device TLS certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("parse device TLS certificate: %w", err)
	}
	certificateDigest := sha256.Sum256(der)
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  transportPrivate,
		Leaf:        leaf,
	}, hex.EncodeToString(certificateDigest[:]), nil
}

func deriveDeviceTLSTransportKey(identityPrivate ed25519.PrivateKey) (*ecdsa.PrivateKey, error) {
	if len(identityPrivate) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: invalid private key length", ErrIdentityMismatch)
	}
	seed := identityPrivate.Seed()
	material := make([]byte, 0, len(deviceTLSTransportKeyContext)+len(seed))
	material = append(material, deviceTLSTransportKeyContext...)
	material = append(material, seed...)
	digest := sha256.Sum256(material)

	curve := elliptic.P256()
	nMinusOne := new(big.Int).Sub(curve.Params().N, big.NewInt(1))
	d := new(big.Int).SetBytes(digest[:])
	d.Mod(d, nMinusOne)
	d.Add(d, big.NewInt(1))

	private := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: curve},
		D:         d,
	}
	private.PublicKey.X, private.PublicKey.Y = curve.ScalarBaseMult(d.Bytes())
	return private, nil
}
