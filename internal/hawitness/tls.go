package hawitness

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"
)

type PeerPin struct {
	ID          string
	Fingerprint string
}

func ServerTLSConfig(identity TransportIdentity, allowedHubs []PeerPin, now func() time.Time) (*tls.Config, error) {
	if len(identity.Certificate.Certificate) == 0 || identity.Certificate.PrivateKey == nil {
		return nil, errors.New("HA witness server certificate is required")
	}
	pins, err := normalizePins(allowedHubs)
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		return nil, errors.New("at least one authorized HA Hub is required")
	}
	if now == nil {
		now = time.Now
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{identity.Certificate},
		ClientAuth:   tls.RequireAnyClientCert,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			certificate, err := parseLeaf(rawCerts)
			if err != nil {
				return err
			}
			id, err := CertificateIdentity(certificate, RoleHub)
			if err != nil {
				return err
			}
			pin, ok := pins[id]
			if !ok {
				return fmt.Errorf("HA Hub %q is not authorized", id)
			}
			return verifyPinnedCertificate(certificate, pin.Fingerprint, now().UTC())
		},
	}, nil
}

func ClientTLSConfig(hubCertificate tls.Certificate, expectedWitness PeerPin, now func() time.Time) (*tls.Config, error) {
	if len(hubCertificate.Certificate) == 0 || hubCertificate.PrivateKey == nil {
		return nil, errors.New("HA Hub client certificate is required")
	}
	expectedWitness.ID = strings.TrimSpace(expectedWitness.ID)
	expectedWitness.Fingerprint = strings.TrimSpace(expectedWitness.Fingerprint)
	if expectedWitness.ID == "" || expectedWitness.Fingerprint == "" {
		return nil, errors.New("HA witness identity and fingerprint are required")
	}
	if now == nil {
		now = time.Now
	}
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		Certificates:       []tls.Certificate{hubCertificate},
		InsecureSkipVerify: true, // Pin verification below replaces public-PKI verification.
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			certificate, err := parseLeaf(rawCerts)
			if err != nil {
				return err
			}
			id, err := CertificateIdentity(certificate, RoleWitness)
			if err != nil {
				return err
			}
			if id != expectedWitness.ID {
				return fmt.Errorf("unexpected HA witness identity %q", id)
			}
			return verifyPinnedCertificate(certificate, expectedWitness.Fingerprint, now().UTC())
		},
	}, nil
}

func normalizePins(input []PeerPin) (map[string]PeerPin, error) {
	pins := make(map[string]PeerPin, len(input))
	for _, pin := range input {
		pin.ID = strings.TrimSpace(pin.ID)
		pin.Fingerprint = strings.TrimSpace(pin.Fingerprint)
		if pin.ID == "" || pin.Fingerprint == "" {
			return nil, errors.New("HA peer identity and fingerprint are required")
		}
		if existing, exists := pins[pin.ID]; exists && existing.Fingerprint != pin.Fingerprint {
			return nil, fmt.Errorf("conflicting HA peer pin for %q", pin.ID)
		}
		pins[pin.ID] = pin
	}
	return pins, nil
}

func parseLeaf(rawCerts [][]byte) (*x509.Certificate, error) {
	if len(rawCerts) != 1 || len(rawCerts[0]) == 0 {
		return nil, errors.New("exactly one HA peer certificate is required")
	}
	certificate, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return nil, fmt.Errorf("parse HA peer certificate: %w", err)
	}
	return certificate, nil
}

func verifyPinnedCertificate(certificate *x509.Certificate, expectedFingerprint string, now time.Time) error {
	if certificate == nil {
		return errors.New("HA peer certificate is missing")
	}
	if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
		return errors.New("HA peer certificate is not currently valid")
	}
	if err := certificate.CheckSignature(certificate.SignatureAlgorithm, certificate.RawTBSCertificate, certificate.Signature); err != nil {
		return fmt.Errorf("HA peer certificate self-signature is invalid: %w", err)
	}
	fingerprint, err := CertificateFingerprint(certificate)
	if err != nil {
		return err
	}
	if !FingerprintMatches(fingerprint, expectedFingerprint) {
		return errors.New("HA peer certificate fingerprint mismatch")
	}
	return nil
}
