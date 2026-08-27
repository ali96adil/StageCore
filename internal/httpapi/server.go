package httpapi

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/companionauth"
)

type Server struct {
	mux           *http.ServeMux
	companionAuth *companionauth.Service
}

type Option func(*Server)

func WithCompanionAuth(service *companionauth.Service) Option {
	return func(s *Server) { s.companionAuth = service }
}

func New(options ...Option) *Server {
	s := &Server{mux: http.NewServeMux()}
	for _, option := range options {
		option(s)
	}
	s.mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "LIVE"})
	})
	s.mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "READY"})
	})
	if s.companionAuth != nil {
		s.registerCompanionSecurityRoutes()
	}
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type pairingRequestBody struct {
	CompanionID       string   `json:"companion_id"`
	DisplayName       string   `json:"display_name"`
	Hostname          string   `json:"hostname"`
	Platform          string   `json:"platform"`
	Architecture      string   `json:"architecture"`
	Version           string   `json:"version"`
	Capabilities      []string `json:"capabilities"`
	PublicKeyAlgorithm string   `json:"public_key_algorithm"`
	PublicKeyBase64   string   `json:"public_key_base64"`
	ClientNonceBase64 string   `json:"client_nonce_base64"`
}

type pairingStatusBody struct {
	RequestID   string `json:"request_id"`
	PairingCode string `json:"pairing_code"`
}

type authChallengeBody struct {
	CompanionID string `json:"companion_id"`
}

type authSessionBody struct {
	CompanionID    string `json:"companion_id"`
	ChallengeID   string `json:"challenge_id"`
	SignatureBase64 string `json:"signature_base64"`
}

func (s *Server) registerCompanionSecurityRoutes() {
	s.mux.HandleFunc("POST /api/v1/companion/pairing/requests", s.handlePairingRequest)
	s.mux.HandleFunc("POST /api/v1/companion/pairing/status", s.handlePairingStatus)
	s.mux.HandleFunc("POST /api/v1/companion/auth/challenges", s.handleAuthChallenge)
	s.mux.HandleFunc("POST /api/v1/companion/auth/sessions", s.handleAuthSession)
}

func (s *Server) handlePairingRequest(w http.ResponseWriter, r *http.Request) {
	if !secureDeviceRequest(r) {
		writeJSON(w, http.StatusUpgradeRequired, map[string]any{"error_code": "SECURE_TRANSPORT_REQUIRED"})
		return
	}
	var body pairingRequestBody
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	receipt, err := s.companionAuth.RequestPairing(r.Context(), companionauth.PairingRequestInput{
		CompanionID: body.CompanionID, DisplayName: body.DisplayName, Hostname: body.Hostname,
		Platform: body.Platform, Architecture: body.Architecture, Version: body.Version,
		Capabilities: body.Capabilities, PublicKeyAlgorithm: body.PublicKeyAlgorithm,
		PublicKeyBase64: body.PublicKeyBase64, ClientNonceBase64: body.ClientNonceBase64,
	})
	if err != nil {
		writeCompanionAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"request_id": receipt.RequestID, "pairing_code": receipt.PairingCode, "expires_at": receipt.ExpiresAt.Format(time.RFC3339Nano),
	})
}

func (s *Server) handlePairingStatus(w http.ResponseWriter, r *http.Request) {
	if !secureDeviceRequest(r) {
		writeJSON(w, http.StatusUpgradeRequired, map[string]any{"error_code": "SECURE_TRANSPORT_REQUIRED"})
		return
	}
	var body pairingStatusBody
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	status, err := s.companionAuth.PairingStatus(r.Context(), body.RequestID, body.PairingCode)
	if err != nil {
		writeCompanionAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status})
}

func (s *Server) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	if !secureDeviceRequest(r) {
		writeJSON(w, http.StatusUpgradeRequired, map[string]any{"error_code": "SECURE_TRANSPORT_REQUIRED"})
		return
	}
	var body authChallengeBody
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	challenge, err := s.companionAuth.BeginAuthentication(r.Context(), body.CompanionID)
	if err != nil {
		writeCompanionAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"challenge_id": challenge.ChallengeID, "nonce_base64": challenge.NonceBase64, "expires_at": challenge.ExpiresAt.Format(time.RFC3339Nano),
	})
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	if !secureDeviceRequest(r) {
		writeJSON(w, http.StatusUpgradeRequired, map[string]any{"error_code": "SECURE_TRANSPORT_REQUIRED"})
		return
	}
	var body authSessionBody
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	credential, err := s.companionAuth.CompleteAuthentication(r.Context(), body.CompanionID, body.ChallengeID, body.SignatureBase64)
	if err != nil {
		writeCompanionAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"session_id": credential.SessionID, "session_token": credential.Token, "expires_at": credential.ExpiresAt.Format(time.RFC3339Nano),
	})
}

func decodeBoundedJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "INVALID_REQUEST"})
		return false
	}
	return true
}

func writeCompanionAuthError(w http.ResponseWriter, err error) {
	code := companionauth.ErrorCode(err)
	status := http.StatusBadRequest
	switch code {
	case companionauth.CodeUnpaired, companionauth.CodePairingNotApproved:
		status = http.StatusForbidden
	case companionauth.CodeRevoked, companionauth.CodeIdentityMismatch, companionauth.CodeProofInvalid:
		status = http.StatusUnauthorized
	case companionauth.CodePairingExpired, companionauth.CodeChallengeInvalid, companionauth.CodeSessionInvalid:
		status = http.StatusGone
	}
	writeJSON(w, status, map[string]any{"error_code": code})
}

func secureDeviceRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	host := strings.TrimSpace(r.RemoteAddr)
	if address, err := netip.ParseAddrPort(host); err == nil {
		return address.Addr().IsLoopback()
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}
