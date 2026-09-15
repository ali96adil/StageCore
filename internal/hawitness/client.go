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
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultClientTimeout = 2 * time.Second

type ClientConfig struct {
	BaseURL            string
	HubCertificate     tls.Certificate
	WitnessID          string
	WitnessFingerprint string
	Timeout            time.Duration
	Now                func() time.Time
}

type Client struct {
	baseURL *url.URL
	http    *http.Client
	hubID   string
	now     func() time.Time
}

type LeaseObservation struct {
	LeaseResponse
	RequestStartedAt time.Time
	ReceivedAt       time.Time
}

type RemoteError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *RemoteError) Error() string {
	if e == nil {
		return "HA witness request failed"
	}
	if e.Code == "" {
		return fmt.Sprintf("HA witness request failed with HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("HA witness %s: %s", e.Code, e.Message)
}

func NewClient(cfg ClientConfig) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("HA witness base URL must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	if baseURL.Path != "" && baseURL.Path != "/" {
		return nil, errors.New("HA witness base URL must not contain a path")
	}
	leaf, err := certificateLeaf(cfg.HubCertificate)
	if err != nil {
		return nil, fmt.Errorf("HA Hub transport certificate: %w", err)
	}
	hubID, err := CertificateIdentity(leaf, RoleHub)
	if err != nil {
		return nil, err
	}
	witnessID := strings.TrimSpace(cfg.WitnessID)
	if witnessID == "" {
		return nil, errors.New("HA witness ID is required")
	}
	witnessFingerprint := strings.TrimSpace(cfg.WitnessFingerprint)
	if err := validateFingerprint(witnessFingerprint); err != nil {
		return nil, fmt.Errorf("HA witness fingerprint: %w", err)
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultClientTimeout
	}
	if timeout <= 0 || timeout > 30*time.Second {
		return nil, errors.New("HA witness client timeout must be greater than zero and at most 30 seconds")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		Certificates:       []tls.Certificate{cfg.HubCertificate},
		InsecureSkipVerify: true, // Exact pinned identity verification is performed below.
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("HA witness server certificate is required")
			}
			certificate := state.PeerCertificates[0]
			peerID, err := CertificateIdentity(certificate, RoleWitness)
			if err != nil {
				return err
			}
			if peerID != witnessID {
				return errors.New("HA witness identity does not match configured witness")
			}
			fingerprint, err := CertificateFingerprint(certificate)
			if err != nil {
				return err
			}
			if !FingerprintMatches(fingerprint, witnessFingerprint) {
				return errors.New("HA witness public-key fingerprint does not match configured pin")
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2: true,
		TLSClientConfig: tlsConfig,
		TLSHandshakeTimeout: timeout,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout: 30 * time.Second,
	}
	return &Client{
		baseURL: baseURL,
		http: &http.Client{Transport: transport, Timeout: timeout},
		hubID: hubID,
		now: cfg.Now,
	}, nil
}

func (c *Client) HubID() string {
	if c == nil {
		return ""
	}
	return c.hubID
}

func (c *Client) Acquire(ctx context.Context) (LeaseObservation, error) {
	observation, err := c.doLease(ctx, http.MethodPost, "/v1/lease/acquire", struct{}{})
	if err != nil {
		return LeaseObservation{}, err
	}
	if err := c.validateOwnedLease(observation); err != nil {
		return LeaseObservation{}, err
	}
	return observation, nil
}

func (c *Client) Renew(ctx context.Context, epoch uint64) (LeaseObservation, error) {
	if epoch == 0 {
		return LeaseObservation{}, errors.New("HA fencing epoch must be greater than zero")
	}
	observation, err := c.doLease(ctx, http.MethodPost, "/v1/lease/renew", epochRequest{Epoch: epoch})
	if err != nil {
		return LeaseObservation{}, err
	}
	if observation.Epoch != epoch {
		return LeaseObservation{}, errors.New("HA witness renewal returned an unexpected fencing epoch")
	}
	if err := c.validateOwnedLease(observation); err != nil {
		return LeaseObservation{}, err
	}
	return observation, nil
}

func (c *Client) Release(ctx context.Context, epoch uint64) error {
	if c == nil || c.http == nil || c.baseURL == nil {
		return errors.New("HA witness client is unavailable")
	}
	if epoch == 0 {
		return errors.New("HA fencing epoch must be greater than zero")
	}
	body, err := json.Marshal(epochRequest{Epoch: epoch})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/v1/lease/release"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("HA witness release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return decodeRemoteError(response)
	}
	return nil
}

func (c *Client) Current(ctx context.Context) (LeaseObservation, error) {
	return c.doLease(ctx, http.MethodGet, "/v1/lease", nil)
}

// ConservativeDeadline converts witness-owned wall-clock lease duration into a
// local monotonic deadline without assuming the Hub and witness clocks are
// synchronized. The remaining duration is measured entirely in witness time,
// then added to the local request-start instant. Because request start precedes
// witness processing, this deadline cannot extend authority past the witness
// expiry merely because of network latency.
func (o LeaseObservation) ConservativeDeadline() (time.Time, error) {
	if o.RequestStartedAt.IsZero() || o.WitnessTime.IsZero() || o.ExpiresAt.IsZero() {
		return time.Time{}, errors.New("HA lease observation is incomplete")
	}
	remaining := o.ExpiresAt.Sub(o.WitnessTime)
	if remaining <= 0 {
		return time.Time{}, errors.New("HA lease observation is already expired")
	}
	return o.RequestStartedAt.Add(remaining), nil
}

func (c *Client) doLease(ctx context.Context, method, path string, payload any) (LeaseObservation, error) {
	if c == nil || c.http == nil || c.baseURL == nil || c.now == nil {
		return LeaseObservation{}, errors.New("HA witness client is unavailable")
	}
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return LeaseObservation{}, err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), body)
	if err != nil {
		return LeaseObservation{}, err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	started := c.now()
	response, err := c.http.Do(request)
	received := c.now()
	if err != nil {
		return LeaseObservation{}, fmt.Errorf("HA witness request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return LeaseObservation{}, decodeRemoteError(response)
	}
	var lease LeaseResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&lease); err != nil {
		return LeaseObservation{}, fmt.Errorf("decode HA witness lease response: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return LeaseObservation{}, fmt.Errorf("decode HA witness lease response: %w", err)
	}
	return LeaseObservation{LeaseResponse: lease, RequestStartedAt: started, ReceivedAt: received}, nil
}

func (c *Client) validateOwnedLease(observation LeaseObservation) error {
	if observation.HolderID != c.hubID || observation.Epoch == 0 || !observation.Active {
		return errors.New("HA witness returned authority for a different or inactive holder")
	}
	if _, err := observation.ConservativeDeadline(); err != nil {
		return err
	}
	return nil
}

func (c *Client) endpoint(path string) string {
	base := *c.baseURL
	base.Path = path
	base.RawPath = ""
	return base.String()
}

func decodeRemoteError(response *http.Response) error {
	remote := &RemoteError{StatusCode: response.StatusCode, Message: http.StatusText(response.StatusCode)}
	var payload ErrorResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxRequestBodyBytes))
	if err := decoder.Decode(&payload); err == nil {
		remote.Code = strings.TrimSpace(payload.Code)
		if strings.TrimSpace(payload.Message) != "" {
			remote.Message = strings.TrimSpace(payload.Message)
		}
	}
	return remote
}

func certificateLeaf(certificate tls.Certificate) (*x509.Certificate, error) {
	if certificate.Leaf != nil {
		return certificate.Leaf, nil
	}
	if len(certificate.Certificate) == 0 {
		return nil, errors.New("certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse transport certificate: %w", err)
	}
	return leaf, nil
}
