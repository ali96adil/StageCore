package domain

import "time"

const CompanionPublicKeyAlgorithm = "P256_X963_SHA256"

type CompanionPairingStatus string

const (
	CompanionPairingPending  CompanionPairingStatus = "PENDING"
	CompanionPairingApproved CompanionPairingStatus = "APPROVED"
	CompanionPairingRejected CompanionPairingStatus = "REJECTED"
	CompanionPairingExpired  CompanionPairingStatus = "EXPIRED"
)

type CompanionPairingRequest struct {
	ID                   string
	CompanionID          string
	PublicKeyAlgorithm   string
	PublicKeyBase64      string
	PublicKeyFingerprint string
	ClientNonceHash      string
	PairingCodeHash      string
	Status               CompanionPairingStatus
	RequestedAt          time.Time
	ExpiresAt            time.Time
	ApprovedAt           *time.Time
	ApprovedBy           string
}

type CompanionDeviceKey struct {
	CompanionID          string
	PublicKeyAlgorithm   string
	PublicKeyBase64      string
	PublicKeyFingerprint string
	PairedAt             time.Time
	RevokedAt            *time.Time
}

type CompanionAuthChallenge struct {
	ID          string
	CompanionID string
	NonceBase64 string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	UsedAt      *time.Time
}

type CompanionRuntimeSession struct {
	ID             string
	CompanionID    string
	CredentialHash string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
}
