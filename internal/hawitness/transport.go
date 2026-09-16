package hawitness

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/halease"
)

const maxResponseBytes = 64 << 10

type LeaseAuthority interface {
	Acquire(context.Context, string) (halease.Lease, error)
	Renew(context.Context, string, uint64) (halease.Lease, error)
	Release(context.Context, string, uint64) error
	Current(context.Context) (halease.Lease, bool, error)
}

type HubTrust map[string]string

type Server struct {
	leases LeaseAuthority
	trust  HubTrust
}

func NewServer(leases LeaseAuthority, trust HubTrust) (*Server, error) {
	if leases == nil {
		return nil, errors.New("HA witness lease authority is required")
	}
	if len(trust) == 0 {
		return nil, errors.New("HA witness requires at least one trusted Hub")
	}
	normalized := make(HubTrust, len(trust))
	for id, fingerprint := range trust {
		id = strings.TrimSpace(id)
		fingerprint = strings.TrimSpace(fingerprint)
		if id == "" || fingerprint == "" {
			return nil, errors.New("trusted Hub identity and fingerprint are required")
		}
		normalized[id] = fingerprint
	}
	return &Server{leases: leases, trust: normalized}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/lease/acquire", s.acquire)
	mux.HandleFunc("POST /v1/lease/renew", s.renew)
	mux.HandleFunc("POST /v1/lease/release", s.release)
	mux.HandleFunc("GET /v1/lease/current", s.current)
	return mux
}

func (s *Server) TLSConfig(identity TransportIdentity) (*tls.Config, error) {
	if len(identity.Certificate.Certificate) == 0 || identity.Certificate.PrivateKey == nil {
		return nil, errors.New("HA witness TLS identity is required")
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{identity.Certificate},
		ClientAuth:   tls.RequireAnyClientCert,
	}, nil
}

func (s *Server) authenticatedHub(r *http.Request) (string, error) {
	if r == nil || r.TLS == nil || len(r.TLS.PeerCertificates) != 1 {
		return "", errors.New("exactly one Hub client certificate is required")
	}
	certificate := r.TLS.PeerCertificates[0]
	hubID, err := CertificateIdentity(certificate, RoleHub)
	if err != nil {
		return "", err
	}
	fingerprint, err := CertificateFingerprint(certificate)
	if err != nil {
		return "", err
	}
	trustedFingerprint, ok := s.trust[hubID]
	if !ok || !FingerprintMatches(fingerprint, trustedFingerprint) {
		return "", errors.New("Hub identity is not trusted by this witness")
	}
	return hubID, nil
}

type leaseResponse struct {
	HolderID  string `json:"holder_id"`
	Epoch     uint64 `json:"epoch"`
	ExpiresAt string `json:"expires_at"`
	Active    bool   `json:"active"`
}

type epochRequest struct {
	Epoch uint64 `json:"epoch"`
}

type errorResponse struct {
	Code string `json:"code"`
}

func (s *Server) acquire(w http.ResponseWriter, r *http.Request) {
	hubID, err := s.authenticatedHub(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "HA_WITNESS_UNAUTHORIZED")
		return
	}
	lease, err := s.leases.Acquire(r.Context(), hubID)
	if err != nil {
		writeLeaseError(w, err)
		return
	}
	writeLease(w, http.StatusOK, lease, true)
}

func (s *Server) renew(w http.ResponseWriter, r *http.Request) {
	hubID, err := s.authenticatedHub(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "HA_WITNESS_UNAUTHORIZED")
		return
	}
	request, err := decodeEpoch(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "HA_WITNESS_INVALID_REQUEST")
		return
	}
	lease, err := s.leases.Renew(r.Context(), hubID, request.Epoch)
	if err != nil {
		writeLeaseError(w, err)
		return
	}
	writeLease(w, http.StatusOK, lease, true)
}

func (s *Server) release(w http.ResponseWriter, r *http.Request) {
	hubID, err := s.authenticatedHub(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "HA_WITNESS_UNAUTHORIZED")
		return
	}
	request, err := decodeEpoch(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "HA_WITNESS_INVALID_REQUEST")
		return
	}
	if err := s.leases.Release(r.Context(), hubID, request.Epoch); err != nil {
		writeLeaseError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) current(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authenticatedHub(r); err != nil {
		writeError(w, http.StatusUnauthorized, "HA_WITNESS_UNAUTHORIZED")
		return
	}
	lease, active, err := s.leases.Current(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "HA_WITNESS_UNAVAILABLE")
		return
	}
	writeLease(w, http.StatusOK, lease, active)
}

func decodeEpoch(body io.ReadCloser) (epochRequest, error) {
	if body == nil {
		return epochRequest{}, errors.New("request body is required")
	}
	defer body.Close()
	decoder := json.NewDecoder(io.LimitReader(body, 4096))
	decoder.DisallowUnknownFields()
	var request epochRequest
	if err := decoder.Decode(&request); err != nil || request.Epoch == 0 {
		return epochRequest{}, errors.New("valid epoch is required")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return epochRequest{}, errors.New("exactly one request object is required")
	}
	return request, nil
}

func writeLease(w http.ResponseWriter, status int, lease halease.Lease, active bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(leaseResponse{
		HolderID: lease.HolderID,
		Epoch: lease.Epoch,
		ExpiresAt: lease.ExpiresAt.UTC().Format(time.RFC3339Nano),
		Active: active,
	})
}

func writeLeaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, halease.ErrLeaseHeld):
		writeError(w, http.StatusConflict, "HA_LEASE_HELD")
	case errors.Is(err, halease.ErrLeaseExpired):
		writeError(w, http.StatusConflict, "HA_LEASE_EXPIRED")
	case errors.Is(err, halease.ErrLeaseNotHeld):
		writeError(w, http.StatusConflict, "HA_LEASE_NOT_HELD")
	case errors.Is(err, halease.ErrInvalidHolder):
		writeError(w, http.StatusBadRequest, "HA_LEASE_INVALID_HOLDER")
	case errors.Is(err, halease.ErrEpochExhausted):
		writeError(w, http.StatusServiceUnavailable, "HA_LEASE_EPOCH_EXHAUSTED")
	default:
		writeError(w, http.StatusServiceUnavailable, "HA_WITNESS_UNAVAILABLE")
	}
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Code: code})
}

type ClientConfig struct {
	BaseURL            string
	HubCertificate     tls.Certificate
	WitnessID          string
	WitnessFingerprint string
	Timeout            time.Duration
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(cfg ClientConfig) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("HA witness client requires an https base URL")
	}
	if len(cfg.HubCertificate.Certificate) == 0 || cfg.HubCertificate.PrivateKey == nil {
		return nil, errors.New("HA witness client requires a Hub certificate")
	}
	witnessID := strings.TrimSpace(cfg.WitnessID)
	witnessFingerprint := strings.TrimSpace(cfg.WitnessFingerprint)
	if witnessID == "" || witnessFingerprint == "" {
		return nil, errors.New("HA witness identity and fingerprint pin are required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		Certificates:       []tls.Certificate{cfg.HubCertificate},
		InsecureSkipVerify: true, // verified below by pinned key + StageCore identity
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) != 1 {
				return errors.New("exactly one HA witness certificate is required")
			}
			certificate := state.PeerCertificates[0]
			id, err := CertificateIdentity(certificate, RoleWitness)
			if err != nil || id != witnessID {
				return errors.New("HA witness identity does not match configured identity")
			}
			fingerprint, err := CertificateFingerprint(certificate)
			if err != nil || !FingerprintMatches(fingerprint, witnessFingerprint) {
				return errors.New("HA witness fingerprint does not match configured pin")
			}
			now := time.Now()
			if now.Before(certificate.NotBefore) || now.After(certificate.NotAfter) {
				return errors.New("HA witness certificate is outside its validity window")
			}
			return nil
		},
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	return &Client{
		baseURL: baseURL,
		http: &http.Client{Transport: transport, Timeout: timeout},
	}, nil
}

func (c *Client) Acquire(ctx context.Context) (halease.Lease, error) {
	return c.doLease(ctx, http.MethodPost, "/v1/lease/acquire", nil)
}

func (c *Client) Renew(ctx context.Context, epoch uint64) (halease.Lease, error) {
	if epoch == 0 {
		return halease.Lease{}, errors.New("valid HA lease epoch is required")
	}
	return c.doLease(ctx, http.MethodPost, "/v1/lease/renew", epochRequest{Epoch: epoch})
}

func (c *Client) Release(ctx context.Context, epoch uint64) error {
	if epoch == 0 {
		return errors.New("valid HA lease epoch is required")
	}
	body, err := json.Marshal(epochRequest{Epoch: epoch})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/lease/release", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("HA witness release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return nil
	}
	return decodeClientError(response)
}

func (c *Client) Current(ctx context.Context) (halease.Lease, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/lease/current", nil)
	if err != nil {
		return halease.Lease{}, false, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return halease.Lease{}, false, fmt.Errorf("HA witness current lease: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return halease.Lease{}, false, decodeClientError(response)
	}
	lease, active, err := decodeLease(response.Body)
	return lease, active, err
}

func (c *Client) doLease(ctx context.Context, method, path string, payload any) (halease.Lease, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return halease.Lease{}, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return halease.Lease{}, err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return halease.Lease{}, fmt.Errorf("HA witness request %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return halease.Lease{}, decodeClientError(response)
	}
	lease, _, err := decodeLease(response.Body)
	return lease, err
}

func decodeLease(body io.Reader) (halease.Lease, bool, error) {
	decoder := json.NewDecoder(io.LimitReader(body, maxResponseBytes))
	decoder.DisallowUnknownFields()
	var response leaseResponse
	if err := decoder.Decode(&response); err != nil {
		return halease.Lease{}, false, fmt.Errorf("decode HA witness lease: %w", err)
	}
	if response.Epoch == 0 && response.HolderID != "" {
		return halease.Lease{}, false, errors.New("HA witness returned invalid lease epoch")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, response.ExpiresAt)
	if err != nil && response.ExpiresAt != "0001-01-01T00:00:00Z" {
		return halease.Lease{}, false, fmt.Errorf("decode HA witness lease expiry: %w", err)
	}
	return halease.Lease{HolderID: response.HolderID, Epoch: response.Epoch, ExpiresAt: expiresAt}, response.Active, nil
}

func decodeClientError(response *http.Response) error {
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	var payload errorResponse
	_ = decoder.Decode(&payload)
	code := strings.TrimSpace(payload.Code)
	if code == "" {
		code = "HA_WITNESS_HTTP_" + strconv.Itoa(response.StatusCode)
	}
	return fmt.Errorf("%s", code)
}

func HubCertificate(id string, privateKey any, now time.Time) (tls.Certificate, error) {
	key, ok := privateKey.([]byte)
	if !ok {
		return tls.Certificate{}, errors.New("Hub transport private key is invalid")
	}
	return newHubCertificateFromBytes(id, key, now)
}

func newHubCertificateFromBytes(id string, privateKey []byte, now time.Time) (tls.Certificate, error) {
	if len(privateKey) != 64 {
		return tls.Certificate{}, errors.New("Hub transport identity requires an Ed25519 private key")
	}
	return NewCertificate(id, RoleHub, privateKey, now)
}

var _ = x509.Certificate{}
