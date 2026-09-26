package deviceexperience

import (
	"context"
	"fmt"
)

// AllocateV2ConnectionGeneration atomically advances the Hub's durable
// singleton. SQLite serializes this UPDATE across processes, including
// overlapping Hub restarts. An unavailable or exhausted sequence fails
// closed: the caller MUST NOT register a v2 socket with a reused generation.
//
// Legacy protocol v1 is intentionally unaffected by this allocation path.
func (r *Repository) AllocateV2ConnectionGeneration(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("%w: v2 generation store unavailable", ErrInvalidState)
	}
	var generation int64
	err := r.db.QueryRowContext(ctx, `
		UPDATE stage_device_v2_connection_sequence
		SET generation = generation + 1
		WHERE singleton = 1 AND generation < 9007199254740991
		RETURNING generation
	`).Scan(&generation)
	if err != nil || generation <= 0 {
		return 0, fmt.Errorf("%w: durable v2 socket generation unavailable: %v", ErrInvalidState, err)
	}
	return generation, nil
}
