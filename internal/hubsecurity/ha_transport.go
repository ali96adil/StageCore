package hubsecurity

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"fmt"
	"os"

	"github.com/ali96adil/StageCore/internal/hawitness"
)

// HATransportCertificate derives the optional HA transport certificate from
// the existing durable Hub identity. The private key never leaves this service;
// HA therefore does not introduce a second Hub identity or a second trust pin.
func (s *Service) HATransportCertificate(ctx context.Context) (tls.Certificate, error) {
	if s == nil {
		return tls.Certificate{}, fmt.Errorf("Hub security service is unavailable")
	}
	identity, err := s.ensureIdentity(ctx)
	if err != nil {
		return tls.Certificate{}, err
	}
	privateRaw, err := os.ReadFile(s.identityKeyPath())
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("read Hub identity key for HA transport: %w", err)
	}
	if len(privateRaw) != ed25519.PrivateKeySize {
		return tls.Certificate{}, fmt.Errorf("%w: invalid private key length", ErrIdentityMismatch)
	}
	certificate, err := hawitness.NewCertificate(identity.HubID, hawitness.RoleHub, ed25519.PrivateKey(privateRaw), s.now().UTC())
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("derive Hub HA transport certificate: %w", err)
	}
	certificateFingerprint, err := hawitness.CertificateFingerprint(certificate.Leaf)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("derive Hub HA transport fingerprint: %w", err)
	}
	if !hawitness.FingerprintMatches(certificateFingerprint, identity.Fingerprint) {
		return tls.Certificate{}, fmt.Errorf("%w: HA transport certificate does not match durable Hub identity", ErrIdentityMismatch)
	}
	return certificate, nil
}
