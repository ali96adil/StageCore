package hubsecurity

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"fmt"
	"os"

	"github.com/ali96adil/StageCore/internal/hawitness"
)

// HATransportCertificate reuses the durable Hub identity key for mutual-TLS
// proof to the optional HA witness. It does not create a second Hub identity.
func (s *Service) HATransportCertificate(ctx context.Context) (Identity, tls.Certificate, error) {
	if s == nil {
		return Identity{}, tls.Certificate{}, fmt.Errorf("Hub security service is unavailable")
	}
	identity, err := s.ensureIdentity(ctx)
	if err != nil {
		return Identity{}, tls.Certificate{}, err
	}
	privateRaw, err := os.ReadFile(s.identityKeyPath())
	if err != nil {
		return Identity{}, tls.Certificate{}, fmt.Errorf("read Hub identity key for HA transport: %w", err)
	}
	if len(privateRaw) != ed25519.PrivateKeySize {
		return Identity{}, tls.Certificate{}, fmt.Errorf("%w: invalid private key length", ErrIdentityMismatch)
	}
	certificate, err := hawitness.NewCertificate(identity.HubID, hawitness.RoleHub, ed25519.PrivateKey(privateRaw), s.now().UTC())
	if err != nil {
		return Identity{}, tls.Certificate{}, fmt.Errorf("create Hub HA transport certificate: %w", err)
	}
	certificateFingerprint, err := hawitness.CertificateFingerprint(certificate.Leaf)
	if err != nil {
		return Identity{}, tls.Certificate{}, err
	}
	if !hawitness.FingerprintMatches(certificateFingerprint, identity.Fingerprint) {
		return Identity{}, tls.Certificate{}, ErrIdentityMismatch
	}
	return identity, certificate, nil
}
