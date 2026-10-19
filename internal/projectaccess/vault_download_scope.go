package projectaccess

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// CheckScopedVaultObjectRead is a QUALIFICATION-ONLY policy for a future
// browser/Owner Project media download path. It is deliberately not registered
// on today's raw-hash Companion Vault route.
//
// One content hash may be referenced by many Project-owned Media Versions.
// Authorization must bind the requesting user, exact Project, Media Version
// and immutable SHA-256 identity together, then recheck the current media.read
// grant. A bare content_hash is never a valid Project access capability.
//
// This method does NOT authenticate a browser or device session, open a file,
// grant a Companion distribution ticket or replace transport/runtime safety.
// Each caller must authenticate its own principal separately. In particular,
// DO NOT attach this to the existing Companion endpoint without a separate
// design for authorized device prefetch and Project/Session binding.
func (s *Service) CheckScopedVaultObjectRead(
	ctx context.Context, userID, expectedProjectID, mediaContentVersionID, contentHash string,
) error {
	if s == nil || s.db == nil {
		return ErrDenied
	}
	userID = strings.TrimSpace(userID)
	expectedProjectID = strings.TrimSpace(expectedProjectID)
	mediaContentVersionID = strings.TrimSpace(mediaContentVersionID)
	// Content hashes are stored as lowercase SHA-256 hexadecimal identities.
	// Do not silently normalize attacker-supplied identifiers.
	if userID == "" || expectedProjectID == "" || mediaContentVersionID == "" ||
		len(contentHash) != 64 || contentHash != strings.ToLower(contentHash) {
		return ErrDenied
	}
	digest, err := hex.DecodeString(contentHash)
	if err != nil || len(digest) != 32 {
		return ErrDenied
	}

	// Resolve through the Project's authoritative Asset -> Content Version ->
	// durable Vault identity. Never look up Project ownership from a hash.
	var storedHash string
	err = s.db.QueryRowContext(ctx, `
		SELECT v.content_hash
		FROM media_content_versions v
		JOIN media_assets a ON a.media_asset_id = v.media_asset_id
		JOIN vault_objects o ON o.content_hash = v.content_hash
		WHERE v.content_version_id = ? AND a.project_id = ?
	`, mediaContentVersionID, expectedProjectID).Scan(&storedHash)
	if errors.Is(err, sql.ErrNoRows) {
		// Same response for a missing Version and a foreign Project Version;
		// do not confirm whether another Show owns a guessed Media ID.
		return ErrDenied
	}
	if err != nil {
		return fmt.Errorf("resolve scoped Vault content version: %w", err)
	}
	if len(storedHash) != 64 || subtle.ConstantTimeCompare([]byte(storedHash), []byte(contentHash)) != 1 {
		return ErrDenied
	}
	// Current, DB-backed grants: project.view alone cannot download media.
	return s.Check(ctx, userID, expectedProjectID, MediaRead)
}
