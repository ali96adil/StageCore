package hawitness

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	RoleHub     = "hub"
	RoleWitness = "witness"

	identityFileName = "ha_witness_identity.ed25519"
)

type TransportIdentity struct {
	ID          string
	Fingerprint string
	Certificate tls.Certificate
}

// LoadOrCreateIdentity owns the witness transport identity. The private key is
// persisted separately from either Hub and is never exposed through the lease
// API. The human/configuration trust anchor is the stable public-key
// fingerprint, so a self-signed certificate may be regenerated without
// changing witness trust.
func LoadOrCreateIdentity(dataRoot string, now func() time.Time) (TransportIdentity, error) {
	dataRoot = strings.TrimSpace(dataRoot)
	if dataRoot == "" {
		return TransportIdentity{}, errors.New("HA witness data root is required")
	}
	if now == nil {
		now = time.Now
	}
	securityDir := filepath.Join(dataRoot, "security")
	if err := os.MkdirAll(securityDir, 0o700); err != nil {
		return TransportIdentity{}, fmt.Errorf("create HA witness security directory: %w", err)
	}
	if err := os.Chmod(securityDir, 0o700); err != nil {
		return TransportIdentity{}, fmt.Errorf("secure HA witness security directory: %w", err)
	}
	keyPath := filepath.Join(securityDir, identityFileName)
	privateKey, err := loadOrCreatePrivateKey(keyPath)
	if err != nil {
		return TransportIdentity{}, err
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	digest := sha256.Sum256(publicKey)
	id := "witness-" + hex.EncodeToString(digest[:16])
	certificate, err := NewCertificate(id, RoleWitness, privateKey, now().UTC())
	if err != nil {
		return TransportIdentity{}, err
	}
	fingerprint, err := CertificateFingerprint(certificate.Leaf)
	if err != nil {
		return TransportIdentity{}, err
	}
	return TransportIdentity{ID: id, Fingerprint: fingerprint, Certificate: certificate}, nil
}

// NewCertificate creates a short-purpose self-signed identity certificate.
// Trust never comes from the self-signature itself: peers pin the Ed25519
// public-key fingerprint and validate the StageCore role/identity URI.
func NewCertificate(id, role string, privateKey ed25519.PrivateKey, now time.Time) (tls.Certificate, error) {
	id = strings.TrimSpace(id)
	role = strings.TrimSpace(role)
	if id == "" || (role != RoleHub && role != RoleWitness) {
		return tls.Certificate{}, errors.New("valid StageCore transport identity and role are required")
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return tls.Certificate{}, errors.New("StageCore transport identity requires an Ed25519 private key")
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate StageCore transport certificate serial: %w", err)
	}
	identityURI := &url.URL{Scheme: "stagecore", Host: role, Path: "/" + id}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{CommonName: "StageCore " + role + " " + id},
		NotBefore: now.Add(-24 * time.Hour),
		NotAfter: now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		URIs: []*url.URL{identityURI},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create StageCore transport certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse StageCore transport certificate: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: privateKey, Leaf: leaf}, nil
}

func CertificateIdentity(certificate *x509.Certificate, role string) (string, error) {
	if certificate == nil {
		return "", errors.New("peer certificate is missing")
	}
	for _, identityURI := range certificate.URIs {
		if identityURI == nil || identityURI.Scheme != "stagecore" || identityURI.Host != role {
			continue
		}
		id := strings.TrimPrefix(identityURI.EscapedPath(), "/")
		decoded, err := url.PathUnescape(id)
		if err != nil || strings.TrimSpace(decoded) == "" {
			continue
		}
		return decoded, nil
	}
	return "", fmt.Errorf("peer certificate does not contain a StageCore %s identity", role)
}

func CertificateFingerprint(certificate *x509.Certificate) (string, error) {
	if certificate == nil {
		return "", errors.New("peer certificate is missing")
	}
	publicKey, ok := certificate.PublicKey.(ed25519.PublicKey)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return "", errors.New("StageCore HA transport requires an Ed25519 public key")
	}
	digest := sha256.Sum256(publicKey)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:]), nil
}

func FingerprintMatches(got, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if len(got) != len(want) || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func loadOrCreatePrivateKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("HA witness identity key permissions %o are too broad", info.Mode().Perm())
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read HA witness identity key: %w", err)
		}
		if len(raw) != ed25519.PrivateKeySize {
			return nil, errors.New("HA witness identity key has invalid length")
		}
		return ed25519.PrivateKey(raw), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect HA witness identity key: %w", err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate HA witness identity key: %w", err)
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create HA witness identity key: %w", err)
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.Write(privateKey); err != nil {
		return nil, fmt.Errorf("write HA witness identity key: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("sync HA witness identity key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close HA witness identity key: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return nil, fmt.Errorf("promote HA witness identity key: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("secure HA witness identity key: %w", err)
	}
	ok = true
	return privateKey, nil
}
