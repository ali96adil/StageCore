package securityaudit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/secretstore"
)

const (
	ResultSuccess  = "SUCCESS"
	ResultRejected = "REJECTED"
	ResultFailed   = "FAILED"
)

type Record struct {
	AuditID       string          `json:"audit_id"`
	EventType     string          `json:"event_type"`
	OccurredAt    time.Time       `json:"occurred_at"`
	ActorUserID   string          `json:"actor_user_id,omitempty"`
	ActorUsername string          `json:"actor_username,omitempty"`
	Source        string          `json:"source,omitempty"`
	ResourceType  string          `json:"resource_type,omitempty"`
	ResourceID    string          `json:"resource_id,omitempty"`
	Result        string          `json:"result"`
	Reason        string          `json:"reason,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Metadata      json.RawMessage `json:"metadata"`
}

type Event struct {
	EventType     string
	ActorUserID   string
	ActorUsername string
	Source        string
	ResourceType  string
	ResourceID    string
	Result        string
	Reason        string
	CorrelationID string
	Metadata      any
}

type Service struct {
	db      *sql.DB
	secrets *secretstore.Service
	now     func() time.Time
}

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func New(database *sql.DB, secrets *secretstore.Service, options ...Option) (*Service, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}
	s := &Service{db: database, secrets: secrets, now: time.Now}
	for _, option := range options {
		option(s)
	}
	return s, nil
}

func (s *Service) Append(ctx context.Context, event Event) (Record, error) {
	event.EventType = strings.TrimSpace(event.EventType)
	if event.EventType == "" || len(event.EventType) > 128 {
		return Record{}, fmt.Errorf("audit event type is required")
	}
	if event.Result != ResultSuccess && event.Result != ResultRejected && event.Result != ResultFailed {
		return Record{}, fmt.Errorf("invalid audit result")
	}

	metadata := json.RawMessage(`{}`)
	if event.Metadata != nil {
		raw, err := json.Marshal(event.Metadata)
		if err != nil {
			return Record{}, fmt.Errorf("encode audit metadata: %w", err)
		}
		metadata = raw
	}
	if s.secrets != nil {
		metadata = s.secrets.RedactJSON(ctx, metadata)
		event.Reason = s.secrets.RedactString(ctx, event.Reason)
	}

	auditID, err := stageid.New()
	if err != nil {
		return Record{}, err
	}
	now := s.now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO security_audit_records
		(audit_id, event_type, occurred_at_us, actor_user_id, actor_username, source,
		 resource_type, resource_id, result, reason, correlation_id, metadata_json)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		auditID, event.EventType, now.UnixMicro(), strings.TrimSpace(event.ActorUserID),
		strings.TrimSpace(event.ActorUsername), strings.TrimSpace(event.Source),
		strings.TrimSpace(event.ResourceType), strings.TrimSpace(event.ResourceID), event.Result,
		strings.TrimSpace(event.Reason), strings.TrimSpace(event.CorrelationID), string(metadata),
	); err != nil {
		return Record{}, fmt.Errorf("append security audit: %w", err)
	}
	return Record{
		AuditID: auditID, EventType: event.EventType, OccurredAt: now,
		ActorUserID: strings.TrimSpace(event.ActorUserID), ActorUsername: strings.TrimSpace(event.ActorUsername),
		Source: strings.TrimSpace(event.Source), ResourceType: strings.TrimSpace(event.ResourceType), ResourceID: strings.TrimSpace(event.ResourceID),
		Result: event.Result, Reason: strings.TrimSpace(event.Reason), CorrelationID: strings.TrimSpace(event.CorrelationID), Metadata: metadata,
	}, nil
}

func (s *Service) List(ctx context.Context, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT audit_id, event_type, occurred_at_us, COALESCE(actor_user_id,''), actor_username,
		       source, resource_type, resource_id, result, reason, correlation_id, metadata_json
		FROM security_audit_records
		ORDER BY occurred_at_us DESC, audit_id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list security audit: %w", err)
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		var record Record
		var occurredUS int64
		var metadata string
		if err := rows.Scan(
			&record.AuditID, &record.EventType, &occurredUS, &record.ActorUserID,
			&record.ActorUsername, &record.Source, &record.ResourceType, &record.ResourceID,
			&record.Result, &record.Reason, &record.CorrelationID, &metadata,
		); err != nil {
			return nil, fmt.Errorf("scan security audit: %w", err)
		}
		record.OccurredAt = time.UnixMicro(occurredUS).UTC()
		record.Metadata = json.RawMessage(metadata)
		records = append(records, record)
	}
	return records, rows.Err()
}
