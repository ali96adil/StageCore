package hawitness

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/halease"
)

const maxRequestBodyBytes = 4096

type LeaseAuthority interface {
	Acquire(context.Context, string) (halease.Lease, error)
	Renew(context.Context, string, uint64) (halease.Lease, error)
	Release(context.Context, string, uint64) error
	Current(context.Context) (halease.Lease, bool, error)
}

type ServerConfig struct {
	AuthorizedHubs map[string]string
	Now            func() time.Time
}

type Server struct {
	authority LeaseAuthority
	allowed   map[string]string
	now       func() time.Time
}

type LeaseResponse struct {
	HolderID    string    `json:"holder_id"`
	Epoch       uint64    `json:"epoch"`
	ExpiresAt   time.Time `json:"expires_at"`
	Active      bool      `json:"active"`
	WitnessTime time.Time `json:"witness_time"`
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type epochRequest struct {
	Epoch uint64 `json:"epoch"`
}

func NewServer(authority LeaseAuthority, cfg ServerConfig) (*Server, error) {
	if authority == nil {
		return nil, errors.New("HA witness lease authority is required")
	}
	if len(cfg.AuthorizedHubs) == 0 {
		return nil, errors.New("at least one authorized HA Hub is required")
	}
	allowed := make(map[string]string, len(cfg.AuthorizedHubs))
	for rawID, rawFingerprint := range cfg.AuthorizedHubs {
		id := strings.TrimSpace(rawID)
		fingerprint := strings.TrimSpace(rawFingerprint)
		if id == "" {
			return nil, errors.New("authorized HA Hub ID is required")
		}
		if err := validateFingerprint(fingerprint); err != nil {
			return nil, fmt.Errorf("authorized HA Hub %q fingerprint: %w", id, err)
		}
		if _, exists := allowed[id]; exists {
			return nil, fmt.Errorf("duplicate authorized HA Hub ID %q", id)
		}
		allowed[id] = fingerprint
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Server{authority: authority, allowed: allowed, now: cfg.Now}, nil
}

// TLSConfig requires a client certificate, then pins its Ed25519 public-key
// fingerprint to the configured HubID. The certificate is self-signed by
// design; authority comes from the explicit pin plus proof-of-possession in the
// TLS handshake, not from the public Web PKI.
func (s *Server) TLSConfig(identity TransportIdentity) (*tls.Config, error) {
	if s == nil {
		return nil, errors.New("HA witness server is unavailable")
	}
	if identity.Certificate.Leaf == nil {
		return nil, errors.New("HA witness TLS certificate is required")
	}
	if _, err := CertificateIdentity(identity.Certificate.Leaf, RoleWitness); err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{identity.Certificate},
		ClientAuth:   tls.RequireAnyClientCert,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("HA witness client certificate is required")
			}
			certificate := state.PeerCertificates[0]
			hubID, err := CertificateIdentity(certificate, RoleHub)
			if err != nil {
				return err
			}
			fingerprint, err := CertificateFingerprint(certificate)
			if err != nil {
				return err
			}
			want, ok := s.allowed[hubID]
			if !ok || !FingerprintMatches(fingerprint, want) {
				return errors.New("HA Hub transport identity is not authorized")
			}
			return nil
		},
	}, nil
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serveHTTP)
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.authority == nil {
		writeError(w, http.StatusServiceUnavailable, "WITNESS_UNAVAILABLE", "HA witness is unavailable")
		return
	}
	hubID, err := authenticatedHubID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "HUB_AUTHENTICATION_REQUIRED", "authenticated HA Hub identity is required")
		return
	}
	if want, ok := s.allowed[hubID]; !ok || len(r.TLS.PeerCertificates) == 0 {
		writeError(w, http.StatusForbidden, "HUB_NOT_AUTHORIZED", "HA Hub is not authorized")
		return
	} else if fingerprint, fingerprintErr := CertificateFingerprint(r.TLS.PeerCertificates[0]); fingerprintErr != nil || !FingerprintMatches(fingerprint, want) {
		writeError(w, http.StatusForbidden, "HUB_NOT_AUTHORIZED", "HA Hub is not authorized")
		return
	}

	switch r.URL.Path {
	case "/v1/lease":
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		s.handleCurrent(w, r)
	case "/v1/lease/acquire":
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		if err := requireEmptyJSONBody(w, r); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		s.handleAcquire(w, r, hubID)
	case "/v1/lease/renew":
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		request, err := decodeEpochRequest(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		s.handleRenew(w, r, hubID, request.Epoch)
	case "/v1/lease/release":
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		request, err := decodeEpochRequest(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		s.handleRelease(w, r, hubID, request.Epoch)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "HA witness endpoint was not found")
	}
}

func (s *Server) handleCurrent(w http.ResponseWriter, r *http.Request) {
	lease, active, err := s.authority.Current(r.Context())
	if err != nil {
		writeAuthorityError(w, err)
		return
	}
	s.writeLease(w, http.StatusOK, lease, active)
}

func (s *Server) handleAcquire(w http.ResponseWriter, r *http.Request, hubID string) {
	lease, err := s.authority.Acquire(r.Context(), hubID)
	if err != nil {
		writeAuthorityError(w, err)
		return
	}
	s.writeLease(w, http.StatusOK, lease, true)
}

func (s *Server) handleRenew(w http.ResponseWriter, r *http.Request, hubID string, epoch uint64) {
	lease, err := s.authority.Renew(r.Context(), hubID, epoch)
	if err != nil {
		writeAuthorityError(w, err)
		return
	}
	s.writeLease(w, http.StatusOK, lease, true)
}

func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request, hubID string, epoch uint64) {
	if err := s.authority.Release(r.Context(), hubID, epoch); err != nil {
		writeAuthorityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeLease(w http.ResponseWriter, status int, lease halease.Lease, active bool) {
	response := LeaseResponse{
		HolderID: lease.HolderID,
		Epoch: lease.Epoch,
		ExpiresAt: lease.ExpiresAt.UTC(),
		Active: active,
		WitnessTime: s.now().UTC(),
	}
	writeJSON(w, status, response)
}

func authenticatedHubID(r *http.Request) (string, error) {
	if r == nil || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return "", errors.New("peer certificate is missing")
	}
	return CertificateIdentity(r.TLS.PeerCertificates[0], RoleHub)
}

func requireEmptyJSONBody(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var payload struct{}
	err := decoder.Decode(&payload)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return errors.New("request body must be empty or an empty JSON object")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func decodeEpochRequest(w http.ResponseWriter, r *http.Request) (epochRequest, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request epochRequest
	if err := decoder.Decode(&request); err != nil {
		return epochRequest{}, errors.New("request must contain a valid fencing epoch")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return epochRequest{}, err
	}
	if request.Epoch == 0 {
		return epochRequest{}, errors.New("fencing epoch must be greater than zero")
	}
	return request, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON value")
	}
	return nil
}

func writeAuthorityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, halease.ErrLeaseHeld):
		writeError(w, http.StatusConflict, "LEASE_HELD", "HA lease is already held")
	case errors.Is(err, halease.ErrLeaseNotHeld):
		writeError(w, http.StatusConflict, "LEASE_NOT_HELD", "HA lease holder or epoch does not match")
	case errors.Is(err, halease.ErrLeaseExpired):
		writeError(w, http.StatusConflict, "LEASE_EXPIRED", "HA lease has expired")
	case errors.Is(err, halease.ErrInvalidHolder):
		writeError(w, http.StatusBadRequest, "INVALID_HOLDER", "HA lease holder is invalid")
	case errors.Is(err, halease.ErrEpochExhausted):
		writeError(w, http.StatusServiceUnavailable, "EPOCH_EXHAUSTED", "HA fencing epoch is exhausted")
	default:
		writeError(w, http.StatusInternalServerError, "WITNESS_INTERNAL", "HA witness could not complete the request")
	}
}

func writeMethodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "HTTP method is not allowed")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Code: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func validateFingerprint(value string) error {
	const prefix = "SHA256:"
	if !strings.HasPrefix(value, prefix) {
		return errors.New("fingerprint must use SHA256 format")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil || len(raw) != 32 {
		return errors.New("fingerprint must contain a 256-bit digest")
	}
	return nil
}
