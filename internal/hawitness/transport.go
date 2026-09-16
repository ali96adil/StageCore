package hawitness

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/halease"
)

const DefaultRequestTimeout = 3 * time.Second

type leaseResponse struct {
	HolderID  string `json:"holder_id"`
	Epoch     uint64 `json:"epoch"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Active    bool   `json:"active"`
}

type epochRequest struct {
	Epoch uint64 `json:"epoch"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type RemoteError struct {
	Status int
	Code   string
}

func (e *RemoteError) Error() string {
	if e == nil {
		return "HA witness request failed"
	}
	return fmt.Sprintf("HA witness request failed: status=%d code=%s", e.Status, e.Code)
}

func NewHandler(leases *halease.Service) (http.Handler, error) {
	if leases == nil {
		return nil, errors.New("HA witness lease service is required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/lease", func(w http.ResponseWriter, r *http.Request) {
		holderID, ok := authenticatedHub(w, r)
		if !ok {
			return
		}
		_ = holderID
		lease, active, err := leases.Current(r.Context())
		if err != nil {
			writeLeaseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, responseForLease(lease, active))
	})
	mux.HandleFunc("POST /v1/lease/acquire", func(w http.ResponseWriter, r *http.Request) {
		holderID, ok := authenticatedHub(w, r)
		if !ok {
			return
		}
		if err := requireEmptyJSON(r); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Code: "INVALID_REQUEST", Message: "request body must be empty or {}"})
			return
		}
		lease, err := leases.Acquire(r.Context(), holderID)
		if err != nil {
			writeLeaseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, responseForLease(lease, true))
	})
	mux.HandleFunc("POST /v1/lease/renew", func(w http.ResponseWriter, r *http.Request) {
		holderID, ok := authenticatedHub(w, r)
		if !ok {
			return
		}
		var input epochRequest
		if err := decodeJSON(r, &input); err != nil || input.Epoch == 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Code: "INVALID_REQUEST", Message: "non-zero epoch is required"})
			return
		}
		lease, err := leases.Renew(r.Context(), holderID, input.Epoch)
		if err != nil {
			writeLeaseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, responseForLease(lease, true))
	})
	mux.HandleFunc("POST /v1/lease/release", func(w http.ResponseWriter, r *http.Request) {
		holderID, ok := authenticatedHub(w, r)
		if !ok {
			return
		}
		var input epochRequest
		if err := decodeJSON(r, &input); err != nil || input.Epoch == 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Code: "INVALID_REQUEST", Message: "non-zero epoch is required"})
			return
		}
		if err := leases.Release(r.Context(), holderID, input.Epoch); err != nil {
			writeLeaseError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux, nil
}

func authenticatedHub(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r == nil || r.TLS == nil || len(r.TLS.PeerCertificates) != 1 {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: "HUB_IDENTITY_REQUIRED", Message: "authenticated HA Hub certificate is required"})
		return "", false
	}
	holderID, err := CertificateIdentity(r.TLS.PeerCertificates[0], RoleHub)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: "HUB_IDENTITY_INVALID", Message: "authenticated HA Hub identity is invalid"})
		return "", false
	}
	return holderID, true
}

func responseForLease(lease halease.Lease, active bool) leaseResponse {
	response := leaseResponse{HolderID: lease.HolderID, Epoch: lease.Epoch, Active: active}
	if !lease.ExpiresAt.IsZero() {
		response.ExpiresAt = lease.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	return response
}

func writeLeaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, halease.ErrLeaseHeld):
		writeJSON(w, http.StatusConflict, errorResponse{Code: "LEASE_HELD", Message: "HA lease is held by another Hub"})
	case errors.Is(err, halease.ErrLeaseNotHeld):
		writeJSON(w, http.StatusConflict, errorResponse{Code: "LEASE_NOT_HELD", Message: "HA lease holder or epoch does not match"})
	case errors.Is(err, halease.ErrLeaseExpired):
		writeJSON(w, http.StatusConflict, errorResponse{Code: "LEASE_EXPIRED", Message: "HA lease has expired"})
	case errors.Is(err, halease.ErrEpochExhausted):
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Code: "EPOCH_EXHAUSTED", Message: "HA fencing epoch is exhausted"})
	case errors.Is(err, halease.ErrInvalidHolder):
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: "HUB_IDENTITY_INVALID", Message: "authenticated HA Hub identity is invalid"})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Code: "WITNESS_UNAVAILABLE", Message: "HA witness authority is unavailable"})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSON(r *http.Request, destination any) error {
	if r == nil || r.Body == nil {
		return io.EOF
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func requireEmptyJSON(r *http.Request) error {
	if r == nil || r.Body == nil {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" {
		return nil
	}
	return errors.New("non-empty request body")
}

type Client struct {
	baseURL  *url.URL
	holderID string
	http     *http.Client
}

func NewClient(baseURL, holderID string, transportTLS *tls.Config, timeout time.Duration) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	holderID = strings.TrimSpace(holderID)
	if baseURL == "" || holderID == "" {
		return nil, errors.New("HA witness URL and Hub identity are required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("HA witness URL must be an https origin")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, errors.New("HA witness URL must not contain a path")
	}
	if transportTLS == nil {
		return nil, errors.New("HA witness TLS configuration is required")
	}
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	parsed.Path = ""
	return &Client{
		baseURL: parsed,
		holderID: holderID,
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{TLSClientConfig: transportTLS.Clone()},
		},
	}, nil
}

func (c *Client) CloseIdleConnections() {
	if c == nil || c.http == nil {
		return
	}
	c.http.CloseIdleConnections()
}

func (c *Client) Acquire(ctx context.Context) (halease.Lease, error) {
	response, err := c.do(ctx, http.MethodPost, "/v1/lease/acquire", []byte("{}"))
	if err != nil {
		return halease.Lease{}, err
	}
	if response.HolderID != c.holderID || !response.Active {
		return halease.Lease{}, errors.New("HA witness returned authority for an unexpected holder")
	}
	return response.lease()
}

func (c *Client) Renew(ctx context.Context, epoch uint64) (halease.Lease, error) {
	payload, err := json.Marshal(epochRequest{Epoch: epoch})
	if err != nil {
		return halease.Lease{}, err
	}
	response, err := c.do(ctx, http.MethodPost, "/v1/lease/renew", payload)
	if err != nil {
		return halease.Lease{}, err
	}
	if response.HolderID != c.holderID || response.Epoch != epoch || !response.Active {
		return halease.Lease{}, errors.New("HA witness returned inconsistent renewed authority")
	}
	return response.lease()
}

func (c *Client) Release(ctx context.Context, epoch uint64) error {
	payload, err := json.Marshal(epochRequest{Epoch: epoch})
	if err != nil {
		return err
	}
	return c.doNoContent(ctx, http.MethodPost, "/v1/lease/release", payload)
}

func (c *Client) Current(ctx context.Context) (halease.Lease, bool, error) {
	response, err := c.do(ctx, http.MethodGet, "/v1/lease", nil)
	if err != nil {
		return halease.Lease{}, false, err
	}
	lease, err := response.lease()
	if err != nil {
		return halease.Lease{}, false, err
	}
	return lease, response.Active, nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (leaseResponse, error) {
	response, err := c.request(ctx, method, path, body)
	if err != nil {
		return leaseResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return leaseResponse{}, decodeRemoteError(response)
	}
	var payload leaseResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return leaseResponse{}, fmt.Errorf("decode HA witness response: %w", err)
	}
	return payload, nil
}

func (c *Client) doNoContent(ctx context.Context, method, path string, body []byte) error {
	response, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return decodeRemoteError(response)
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	if c == nil || c.http == nil || c.baseURL == nil {
		return nil, errors.New("HA witness client is unavailable")
	}
	endpoint := *c.baseURL
	endpoint.Path = path
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("HA witness request: %w", err)
	}
	return response, nil
}

func decodeRemoteError(response *http.Response) error {
	remote := errorResponse{Code: "WITNESS_REQUEST_FAILED"}
	_ = json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&remote)
	return &RemoteError{Status: response.StatusCode, Code: strings.TrimSpace(remote.Code)}
}

func (r leaseResponse) lease() (halease.Lease, error) {
	lease := halease.Lease{HolderID: strings.TrimSpace(r.HolderID), Epoch: r.Epoch}
	if r.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339Nano, r.ExpiresAt)
		if err != nil {
			return halease.Lease{}, fmt.Errorf("invalid HA witness expiry: %w", err)
		}
		lease.ExpiresAt = expiresAt.UTC()
	}
	return lease, nil
}
