package deviceexperience

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

// LiveLightingObservationGate is a source-only, read-only challenge lifecycle.
// The owning authenticated device transport MUST issue challenges on its
// current socket and validate the reply origin before calling Complete.
// It is deliberately not wired to a command transport or runtime READY.
type LiveLightingObservationGate struct {
	mu      sync.Mutex
	pending map[string]pendingLiveLightingObservation
	now     func() time.Time
}

type pendingLiveLightingObservation struct {
	scope     LiveLightingScope
	challenge string
	expiresAt time.Time
}

var ErrLiveObservationScope = errors.New("invalid live lighting observation scope")

const maxLiveObservationTTL = 30 * time.Second

func NewLiveLightingObservationGate() *LiveLightingObservationGate {
	return &LiveLightingObservationGate{
		pending: make(map[string]pendingLiveLightingObservation),
		now:     time.Now,
	}
}

// Begin returns a fresh unpredictable, one-use challenge. Begin must run for
// *every* reconnect (even the same cue), and for each explicit verification.
// Issuing a new challenge invalidates the previous one for the device. The
// caller must separately verify device identity, active session, assignment,
// current authenticated socket, snapshot and command authority.
func (g *LiveLightingObservationGate) Begin(deviceID string, scope LiveLightingScope, ttl time.Duration) (string, error) {
	if g == nil || strings.TrimSpace(deviceID) == "" ||
		strings.TrimSpace(scope.ProjectID) == "" ||
		strings.TrimSpace(scope.SessionID) == "" ||
		strings.TrimSpace(scope.RuntimeSnapshotID) == "" ||
		scope.AssignmentEpoch <= 0 || scope.ConnectionGeneration <= 0 ||
		scope.DesiredRevision == 0 || ttl <= 0 || ttl > maxLiveObservationTTL {
		return "", ErrLiveObservationScope
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err // fail closed if secure entropy is unavailable
	}
	challenge := hex.EncodeToString(token[:])
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending == nil {
		g.pending = make(map[string]pendingLiveLightingObservation)
	}
	g.pending[strings.TrimSpace(deviceID)] = pendingLiveLightingObservation{
		scope: scope, challenge: challenge, expiresAt: g.now().Add(ttl),
	}
	return challenge, nil
}

// Cancel invalidates the current challenge on disconnect/authorization
// revocation. Transport owners must call it after closing that exact socket;
// Begin on a replacement socket also invalidates the old challenge.
func (g *LiveLightingObservationGate) Cancel(deviceID string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	delete(g.pending, strings.TrimSpace(deviceID))
	g.mu.Unlock()
}

// Complete consumes a matching challenge exactly once, comparing a new report
// to the *current* Hub scope. A late old-socket reply cannot consume a newer
// challenge. The result is informational; callers MUST recheck scope before
// any eventual correction and MUST NOT infer physical DMX/LED measurements.
func (g *LiveLightingObservationGate) Complete(
	deviceID string, current LiveLightingScope, desired map[int]uint8,
	observed LiveLightingObservation, commandAuthorityGranted bool,
) LiveLightingComparison {
	blocked := func(reason string) LiveLightingComparison {
		return LiveLightingComparison{Status: LiveLightingBlocked, Reason: reason}
	}
	if g == nil || strings.TrimSpace(deviceID) == "" {
		return blocked("observation gate unavailable")
	}
	deviceID = strings.TrimSpace(deviceID)
	g.mu.Lock()
	pending, found := g.pending[deviceID]
	if !found {
		g.mu.Unlock()
		return blocked("no pending fresh observation for device")
	}
	if pending.challenge != observed.Challenge {
		g.mu.Unlock()
		return blocked("stale or mismatched observation challenge")
	}
	// Consume the exact challenge regardless of success: missing fields,
	// blocked assignment, and duplicate replies must never create MATCH.
	delete(g.pending, deviceID)
	expired := !g.now().Before(pending.expiresAt)
	g.mu.Unlock()
	if expired {
		return blocked("fresh observation deadline expired")
	}
	if pending.scope != current {
		return blocked("Hub session, cue, revision or assignment changed while observing")
	}
	return CompareLiveLighting(current, desired, observed, pending.challenge, commandAuthorityGranted)
}
