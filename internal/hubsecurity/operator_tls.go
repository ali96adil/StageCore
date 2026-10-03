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
	"strings"
)

const operatorTLSTransportKeyContext = "StageCore Operator TLS P-256 v1\x00"

// OperatorTLSCertificate returns the deterministic self-signed certificate used
// by the browser-facing Stage-LAN Operator HTTPS listener. It is deliberately
// domain-separated from the device-gateway TLS key/certificate so adding or
// rotating browser transport cannot silently change Companion/ESP TLS pins.
func (s *Service) OperatorTLSCertificate(ctx context.Context) (tls.Certificate, string, string, error) {
	if s == nil {
		return tls.Certificate{}, "", "", fmt.Errorf("Hub security service is required")
	}
	identity, err := s.ensureIdentity(ctx)
	if err != nil {
		return tls.Certificate{}, "", "", err
	}
	privateBytes, err := os.ReadFile(s.identityKeyPath())
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("read Hub identity key for Operator TLS: %w", err)
	}
	if len(privateBytes) != ed25519.PrivateKeySize {
		return tls.Certificate{}, "", "", fmt.Errorf("%w: invalid private key length", ErrIdentityMismatch)
	}
	identityPrivate := ed25519.PrivateKey(privateBytes)
	identityPublic := identityPrivate.Public().(ed25519.PublicKey)
	if fingerprint(identityPublic) != identity.Fingerprint {
		return tls.Certificate{}, "", "", ErrIdentityMismatch
	}

	transportPrivate, err := deriveOperatorTLSTransportKey(identityPrivate)
	if err != nil {
		return tls.Certificate{}, "", "", err
	}
	host := operatorTLSHostName(identity.HubID)

	idDigest := sha256.Sum256([]byte("operator:" + identity.HubID))
	serial := new(big.Int).SetBytes(idDigest[:16])
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	transportPublicDER, err := x509.MarshalPKIXPublicKey(&transportPrivate.PublicKey)
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("marshal Operator TLS public key: %w", err)
	}
	keyDigest := sha256.Sum256(transportPublicDER)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"StageCore"},
			CommonName:   host,
		},
		DNSNames:              []string{host},
		NotBefore:             deviceCertificateNotBefore,
		NotAfter:              deviceCertificateNotAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		SubjectKeyId:          append([]byte(nil), keyDigest[:20]...),
		AuthorityKeyId:        append([]byte(nil), keyDigest[:20]...),
	}
	signer := deterministicECDSASigner{key: transportPrivate}
	der, err := x509.CreateCertificate(nil, template, template, &transportPrivate.PublicKey, signer)
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("create Operator TLS certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("parse Operator TLS certificate: %w", err)
	}
	digest := sha256.Sum256(der)
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  transportPrivate,
		Leaf:        leaf,
	}, host, hex.EncodeToString(digest[:]), nil
}

func deriveOperatorTLSTransportKey(identityPrivate ed25519.PrivateKey) (*ecdsa.PrivateKey, error) {
	if len(identityPrivate) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: invalid private key length", ErrIdentityMismatch)
	}
	seed := identityPrivate.Seed()
	material := make([]byte, 0, len(operatorTLSTransportKeyContext)+len(seed))
	material = append(material, operatorTLSTransportKeyContext...)
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

func operatorTLSHostName(hubID string) string {
	shortID := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(hubID)), "-", "")
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	return "stagecore-" + shortID + ".local"
}
