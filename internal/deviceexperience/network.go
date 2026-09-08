package deviceexperience

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	stageid "github.com/ali96adil/StageCore/internal/id"
)

func (r *Repository) RecordNetworkObservation(ctx context.Context, observation NetworkObservation) (NetworkObservation, error) {
	observation.TargetKind = strings.TrimSpace(observation.TargetKind)
	observation.TargetID = strings.TrimSpace(observation.TargetID)
	observation.TransportState = strings.TrimSpace(observation.TransportState)
	observation.Address = strings.TrimSpace(observation.Address)
	observation.ErrorCode = strings.TrimSpace(observation.ErrorCode)
	if observation.TargetID == "" || !validTargetKind(observation.TargetKind) || !validReachability(observation.Reachability) {
		return NetworkObservation{}, ErrInvalidState
	}
	if observation.ID == "" {
		id, err := stageid.New()
		if err != nil {
			return NetworkObservation{}, err
		}
		observation.ID = id
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = r.now().UTC()
	} else {
		observation.ObservedAt = observation.ObservedAt.UTC()
	}
	if observation.TransportState == "" {
		observation.TransportState = "UNKNOWN"
	}
	observation.Details = normalizeJSON(observation.Details, `{}`)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO network_observations
		(observation_id, target_kind, target_id, observed_at_us, reachability, transport_state,
		 latency_ms, jitter_ms, address, error_code, details_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, observation.ID, observation.TargetKind, observation.TargetID, observation.ObservedAt.UnixMicro(),
		observation.Reachability, observation.TransportState, observation.LatencyMS, observation.JitterMS,
		observation.Address, observation.ErrorCode, string(observation.Details))
	if err != nil {
		return NetworkObservation{}, fmt.Errorf("record network observation: %w", err)
	}
	return observation, nil
}

func (r *Repository) Cockpit(ctx context.Context, staleAfter time.Duration) ([]CockpitTarget, error) {
	if staleAfter <= 0 {
		staleAfter = 15 * time.Second
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT observation_id, target_kind, target_id, observed_at_us, reachability, transport_state,
		       latency_ms, jitter_ms, address, error_code, details_json
		FROM network_observations
		ORDER BY observed_at_us DESC, observation_id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("read network cockpit observations: %w", err)
	}
	defer rows.Close()
	now := r.now().UTC()
	seen := map[string]bool{}
	var out []CockpitTarget
	for rows.Next() {
		var o NetworkObservation
		var observedUS int64
		var latency, jitter sql.NullFloat64
		var details string
		if err := rows.Scan(&o.ID, &o.TargetKind, &o.TargetID, &observedUS, &o.Reachability,
			&o.TransportState, &latency, &jitter, &o.Address, &o.ErrorCode, &details); err != nil {
			return nil, err
		}
		key := o.TargetKind + "\x00" + o.TargetID
		if seen[key] {
			continue
		}
		seen[key] = true
		o.ObservedAt = time.UnixMicro(observedUS).UTC()
		if latency.Valid {
			value := latency.Float64
			o.LatencyMS = &value
		}
		if jitter.Valid {
			value := jitter.Float64
			o.JitterMS = &value
		}
		o.Details = json.RawMessage(details)
		stale := now.Sub(o.ObservedAt) > staleAfter
		readiness, reason := classifyObservation(o, stale)
		out = append(out, CockpitTarget{TargetKind: o.TargetKind, TargetID: o.TargetID, Readiness: readiness, ReasonCode: reason, Observation: o, Stale: stale})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TargetKind == out[j].TargetKind {
			return out[i].TargetID < out[j].TargetID
		}
		return out[i].TargetKind < out[j].TargetKind
	})
	return out, rows.Err()
}

func (r *Repository) PruneNetworkObservations(ctx context.Context, keepPerTarget int) error {
	if keepPerTarget < 1 {
		return ErrInvalidState
	}
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM network_observations
		WHERE observation_id IN (
			SELECT observation_id FROM (
				SELECT observation_id,
				       ROW_NUMBER() OVER (PARTITION BY target_kind, target_id ORDER BY observed_at_us DESC, observation_id DESC) AS rn
				FROM network_observations
			) WHERE rn > ?
		)
	`, keepPerTarget)
	if err != nil {
		return fmt.Errorf("prune network observations: %w", err)
	}
	return nil
}

func classifyObservation(o NetworkObservation, stale bool) (Readiness, string) {
	if stale {
		return ReadinessWarning, "STALE_OBSERVATION"
	}
	if o.Reachability == Unreachable {
		return ReadinessWarning, "TARGET_UNREACHABLE"
	}
	if o.ErrorCode != "" {
		return ReadinessWarning, o.ErrorCode
	}
	if strings.EqualFold(o.TransportState, "REVOKED") || strings.EqualFold(o.TransportState, "IDENTITY_CONFLICT") {
		return ReadinessBlocker, strings.ToUpper(o.TransportState)
	}
	if o.Reachability == ReachUnknown {
		return ReadinessAdvisory, "REACHABILITY_NOT_MEASURED"
	}
	if o.LatencyMS != nil && *o.LatencyMS >= 100 {
		return ReadinessWarning, "LATENCY_ELEVATED"
	}
	if o.JitterMS != nil && *o.JitterMS >= 30 {
		return ReadinessWarning, "JITTER_ELEVATED"
	}
	return ReadinessReady, "READY"
}
