package companionauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	defaultPairingTTL = 5 * time.Minute
	defaultChallengeTTL = 30 * time.Second
	defaultSessionTTL = 15 * time.Minute
)

type Code string

const (
	CodeInvalidRequest       Code = "PAIRING_REQUEST_INVALID"
	CodeUnpaired             Code = "COMPANION_UNPAIRED"
	CodePairingExpired       Code = "PAIRING_REQUEST_EXPIRED"
	CodePairingNotApproved   Code = "PAIRING_NOT_APPROVED"
	CodeApprovalUnauthorized Code = "PAIRING_APPROVAL_UNAUTHORIZED"
	CodeRevoked              Code = "COMPANION_REVOKED"
	CodeIdentityMismatch     Code = "COMPANION_IDENTITY_MISMATCH"
	CodeChallengeInvalid     Code = "AUTH_CHALLENGE_INVALID"
	CodeProofInvalid         Code = "AUTH_PROOF_INVALID"
	CodeSessionInvalid       Code = "RUNTIME_SESSION_INVALID"
)

type Error struct {
	Code Code
	Err  error
}

func (e *Error) Error() string {
	if e == nil || e.Err == nil {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func ErrorCode(err error) Code {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return CodeInvalidRequest
}

type Service struct {
	store        *store.Store
	now          func() time.Time
	pairingTTL   time.Duration
	challengeTTL time.Duration
	sessionTTL   time.Duration
}

func New(s *store.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		store: s, now: now,
		pairingTTL: defaultPairingTTL, challengeTTL: defaultChallengeTTL, sessionTTL: defaultSessionTTL,
	}
}

type PairingRequestInput struct {
	CompanionID       string
	DisplayName       string
	Hostname          string
	Platform          string
	Architecture      string
	Version           string
	Capabilities      []string
	PublicKeyAlgorithm string
	PublicKeyBase64   string
	ClientNonceBase64 string
}

type PairingReceipt struct {
	RequestID  string
	PairingCode string
	ExpiresAt  time.Time
}

type Approval struct {
	Actor      string
	Authorized bool
}

type AuthChallenge struct {
	ChallengeID string
	NonceBase64 string
	ExpiresAt   time.Time
}

type RuntimeSessionCredential struct {
	SessionID   string
	Token       string
	ExpiresAt   time.Time
}

func (s *Service) RequestPairing(ctx context.Context, input PairingRequestInput) (PairingReceipt, error) {
	if s == nil || s.store == nil {
		return PairingReceipt{}, &Error{Code: CodeInvalidRequest, Err: errors.New("pairing service unavailable")}
	}
	input.CompanionID = strings.TrimSpace(input.CompanionID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.PublicKeyAlgorithm = strings.TrimSpace(input.PublicKeyAlgorithm)
	if input.CompanionID == "" || input.DisplayName == "" || input.PublicKeyAlgorithm != domain.CompanionPublicKeyAlgorithm {
		return PairingReceipt{}, &Error{Code: CodeInvalidRequest, Err: errors.New("companion identity, display name and supported key algorithm are required")}
	}
	_, publicKeyBytes, err := parsePublicKey(input.PublicKeyBase64)
	if err != nil {
		return PairingReceipt{}, &Error{Code: CodeInvalidRequest, Err: errors.New("invalid Companion public key")}
	}
	nonce, err := base64.StdEncoding.DecodeString(strings.TrimSpace(input.ClientNonceBase64))
	if err != nil || len(nonce) < 16 {
		return PairingReceipt{}, &Error{Code: CodeInvalidRequest, Err: errors.New("client nonce must contain at least 128 random bits")}
	}

	companion, err := s.store.GetCompanion(ctx, input.CompanionID)
	if errors.Is(err, domain.ErrNotFound) {
		companion, err = s.store.RegisterCompanion(ctx, store.RegisterCompanionParams{
			CompanionID: input.CompanionID, DisplayName: input.DisplayName, Hostname: input.Hostname,
			Platform: input.Platform, Architecture: input.Architecture, Version: input.Version,
			Capabilities: input.Capabilities,
		})
	}
	if err != nil {
		return PairingReceipt{}, &Error{Code: CodeInvalidRequest, Err: err}
	}
	if companion.TrustState == domain.CompanionRevoked {
		return PairingReceipt{}, &Error{Code: CodeRevoked, Err: errors.New("revoked identity cannot request authority")}
	}
	if companion.TrustState == domain.CompanionTrusted {
		trustedKey, keyErr := s.store.GetCompanionDeviceKey(ctx, companion.ID)
		if keyErr != nil || subtle.ConstantTimeCompare([]byte(trustedKey.PublicKeyBase64), []byte(strings.TrimSpace(input.PublicKeyBase64))) != 1 {
			return PairingReceipt{}, &Error{Code: CodeIdentityMismatch, Err: errors.New("trusted Companion key does not match")}
		}
		return PairingReceipt{}, &Error{Code: CodePairingNotApproved, Err: errors.New("Companion is already paired; authenticate instead")}
	}

	code, err := randomPairingCode()
	if err != nil {
		return PairingReceipt{}, &Error{Code: CodeInvalidRequest, Err: err}
	}
	now := s.now().UTC()
	expiresAt := now.Add(s.pairingTTL)
	publicFingerprint := sha256.Sum256(publicKeyBytes)
	nonceHash := sha256.Sum256(nonce)
	codeHash := sha256.Sum256([]byte(code))
	request, err := s.store.CreateCompanionPairingRequest(ctx, store.CreateCompanionPairingRequestParams{
		CompanionID: companion.ID, PublicKeyAlgorithm: input.PublicKeyAlgorithm,
		PublicKeyBase64: strings.TrimSpace(input.PublicKeyBase64), PublicKeyFingerprint: hex.EncodeToString(publicFingerprint[:]),
		ClientNonceHash: hex.EncodeToString(nonceHash[:]), PairingCodeHash: hex.EncodeToString(codeHash[:]), ExpiresAt: expiresAt,
	})
	if err != nil {
		return PairingReceipt{}, &Error{Code: CodeInvalidRequest, Err: err}
	}
	_ = s.store.AppendCompanionSecurityEvent(ctx, companion.ID, "companion.pairing.requested", "", "PENDING", "", now)
	return PairingReceipt{RequestID: request.ID, PairingCode: code, ExpiresAt: expiresAt}, nil
}

func (s *Service) PairingStatus(ctx context.Context, requestID, pairingCode string) (domain.CompanionPairingStatus, error) {
	request, err := s.store.GetCompanionPairingRequest(ctx, strings.TrimSpace(requestID))
	if err != nil {
		return "", &Error{Code: CodeInvalidRequest, Err: err}
	}
	if !pairingCodeMatches(request.PairingCodeHash, pairingCode) {
		return "", &Error{Code: CodeInvalidRequest, Err: errors.New("pairing request proof is invalid")}
	}
	if request.Status == domain.CompanionPairingPending && !s.now().UTC().Before(request.ExpiresAt) {
		_ = s.store.MarkCompanionPairingExpired(ctx, request.ID)
		return domain.CompanionPairingExpired, nil
	}
	return request.Status, nil
}

func (s *Service) ApprovePairing(ctx context.Context, requestID, pairingCode string, approval Approval) (domain.CompanionDeviceKey, error) {
	if !approval.Authorized || strings.TrimSpace(approval.Actor) == "" {
		return domain.CompanionDeviceKey{}, &Error{Code: CodeApprovalUnauthorized, Err: errors.New("authorized operator approval is required")}
	}
	request, err := s.store.GetCompanionPairingRequest(ctx, strings.TrimSpace(requestID))
	if err != nil {
		return domain.CompanionDeviceKey{}, &Error{Code: CodeInvalidRequest, Err: err}
	}
	if !pairingCodeMatches(request.PairingCodeHash, pairingCode) {
		_ = s.store.AppendCompanionSecurityEvent(ctx, request.CompanionID, "companion.pairing.approval", approval.Actor, "DENIED", "invalid_code", s.now().UTC())
		return domain.CompanionDeviceKey{}, &Error{Code: CodePairingNotApproved, Err: errors.New("pairing code is invalid")}
	}
	if !s.now().UTC().Before(request.ExpiresAt) {
		_ = s.store.MarkCompanionPairingExpired(ctx, request.ID)
		return domain.CompanionDeviceKey{}, &Error{Code: CodePairingExpired, Err: errors.New("pairing request expired")}
	}
	codeHash := sha256.Sum256([]byte(strings.TrimSpace(pairingCode)))
	key, err := s.store.ApproveCompanionPairing(ctx, request.ID, hex.EncodeToString(codeHash[:]), approval.Actor, s.now().UTC())
	if err != nil {
		return domain.CompanionDeviceKey{}, &Error{Code: CodePairingNotApproved, Err: err}
	}
	_ = s.store.AppendCompanionSecurityEvent(ctx, request.CompanionID, "companion.pairing.approved", approval.Actor, "APPROVED", "", s.now().UTC())
	return key, nil
}

func (s *Service) BeginAuthentication(ctx context.Context, companionID string) (AuthChallenge, error) {
	companion, err := s.store.GetCompanion(ctx, strings.TrimSpace(companionID))
	if err != nil || companion.TrustState == domain.CompanionUntrusted {
		return AuthChallenge{}, &Error{Code: CodeUnpaired, Err: errors.New("Companion is not paired")}
	}
	if companion.TrustState == domain.CompanionRevoked {
		return AuthChallenge{}, &Error{Code: CodeRevoked, Err: errors.New("Companion identity is revoked")}
	}
	key, err := s.store.GetCompanionDeviceKey(ctx, companion.ID)
	if err != nil || key.RevokedAt != nil {
		return AuthChallenge{}, &Error{Code: CodeRevoked, Err: errors.New("trusted Companion key is unavailable")}
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return AuthChallenge{}, &Error{Code: CodeChallengeInvalid, Err: err}
	}
	expiresAt := s.now().UTC().Add(s.challengeTTL)
	challenge, err := s.store.CreateCompanionAuthChallenge(ctx, companion.ID, base64.StdEncoding.EncodeToString(nonce), expiresAt)
	if err != nil {
		return AuthChallenge{}, &Error{Code: CodeChallengeInvalid, Err: err}
	}
	return AuthChallenge{ChallengeID: challenge.ID, NonceBase64: challenge.NonceBase64, ExpiresAt: expiresAt}, nil
}

func (s *Service) CompleteAuthentication(ctx context.Context, companionID, challengeID, signatureBase64 string) (RuntimeSessionCredential, error) {
	companion, err := s.store.GetCompanion(ctx, strings.TrimSpace(companionID))
	if err != nil || companion.TrustState != domain.CompanionTrusted {
		code := CodeUnpaired
		if err == nil && companion.TrustState == domain.CompanionRevoked {
			code = CodeRevoked
		}
		return RuntimeSessionCredential{}, &Error{Code: code, Err: errors.New("Companion is not trusted")}
	}
	challenge, err := s.store.GetCompanionAuthChallenge(ctx, strings.TrimSpace(challengeID))
	if err != nil || challenge.CompanionID != companion.ID || challenge.UsedAt != nil || !s.now().UTC().Before(challenge.ExpiresAt) {
		return RuntimeSessionCredential{}, &Error{Code: CodeChallengeInvalid, Err: errors.New("authentication challenge is invalid or expired")}
	}
	key, err := s.store.GetCompanionDeviceKey(ctx, companion.ID)
	if err != nil || key.RevokedAt != nil {
		return RuntimeSessionCredential{}, &Error{Code: CodeRevoked, Err: errors.New("Companion key is revoked")}
	}
	publicKey, _, err := parsePublicKey(key.PublicKeyBase64)
	if err != nil {
		return RuntimeSessionCredential{}, &Error{Code: CodeIdentityMismatch, Err: errors.New("trusted public key is invalid")}
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signatureBase64))
	if err != nil {
		return RuntimeSessionCredential{}, &Error{Code: CodeProofInvalid, Err: errors.New("authentication proof is invalid")}
	}
	message := AuthenticationMessage(companion.ID, challenge.ID, challenge.NonceBase64)
	digest := sha256.Sum256(message)
	if !ecdsa.VerifyASN1(publicKey, digest[:], signature) {
		_ = s.store.AppendCompanionSecurityEvent(ctx, companion.ID, "companion.authentication", "", "DENIED", "invalid_proof", s.now().UTC())
		return RuntimeSessionCredential{}, &Error{Code: CodeProofInvalid, Err: errors.New("private key proof failed")}
	}
	if err := s.store.MarkCompanionAuthChallengeUsed(ctx, challenge.ID, s.now().UTC()); err != nil {
		return RuntimeSessionCredential{}, &Error{Code: CodeChallengeInvalid, Err: err}
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return RuntimeSessionCredential{}, &Error{Code: CodeSessionInvalid, Err: err}
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenHash := sha256.Sum256([]byte(token))
	expiresAt := s.now().UTC().Add(s.sessionTTL)
	session, err := s.store.CreateCompanionRuntimeSession(ctx, companion.ID, hex.EncodeToString(tokenHash[:]), expiresAt)
	if err != nil {
		return RuntimeSessionCredential{}, &Error{Code: CodeSessionInvalid, Err: err}
	}
	_ = s.store.AppendCompanionSecurityEvent(ctx, companion.ID, "companion.authentication", "", "AUTHENTICATED", "", s.now().UTC())
	return RuntimeSessionCredential{SessionID: session.ID, Token: token, ExpiresAt: expiresAt}, nil
}

func (s *Service) ValidateRuntimeSession(ctx context.Context, token string) (domain.CompanionRuntimeSession, error) {
	tokenHash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	session, err := s.store.FindCompanionRuntimeSessionByCredentialHash(ctx, hex.EncodeToString(tokenHash[:]))
	if err != nil || session.RevokedAt != nil || !s.now().UTC().Before(session.ExpiresAt) {
		return domain.CompanionRuntimeSession{}, &Error{Code: CodeSessionInvalid, Err: errors.New("runtime session is invalid or expired")}
	}
	companion, err := s.store.GetCompanion(ctx, session.CompanionID)
	if err != nil || companion.TrustState != domain.CompanionTrusted {
		return domain.CompanionRuntimeSession{}, &Error{Code: CodeRevoked, Err: errors.New("Companion trust is no longer valid")}
	}
	return session, nil
}

func (s *Service) Revoke(ctx context.Context, companionID, actor, reason string, authorized bool) error {
	if !authorized || strings.TrimSpace(actor) == "" {
		return &Error{Code: CodeApprovalUnauthorized, Err: errors.New("authorized operator revocation is required")}
	}
	if err := s.store.RevokeCompanionDevice(ctx, strings.TrimSpace(companionID), s.now().UTC()); err != nil {
		return err
	}
	_ = s.store.AppendCompanionSecurityEvent(ctx, companionID, "companion.revoked", actor, "REVOKED", strings.TrimSpace(reason), s.now().UTC())
	return nil
}

func AuthenticationMessage(companionID, challengeID, nonceBase64 string) []byte {
	return []byte("StageCore Companion Authentication v1\n" + companionID + "\n" + challengeID + "\n" + nonceBase64)
}

func pairingCodeMatches(storedHash, code string) bool {
	hash := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return subtle.ConstantTimeCompare([]byte(storedHash), []byte(hex.EncodeToString(hash[:]))) == 1
}

func randomPairingCode() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

func parsePublicKey(encoded string) (*ecdsa.PublicKey, []byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, nil, err
	}
	x, y := elliptic.Unmarshal(elliptic.P256(), raw)
	if x == nil || y == nil {
		return nil, nil, errors.New("public key is not a P-256 X9.63 point")
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, raw, nil
}
