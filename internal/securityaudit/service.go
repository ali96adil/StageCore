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

// PreparedEvent has already been validated, redacted, timestamped and
// assigned an immutable ID. It can be persisted inside a DB transaction
// without consulting the Secret Store (which would deadlock a single-connection
// SQLite database while a transaction is holding the connection).
type PreparedEvent struct {
	record Record
}

// Prepare performs all non-transactional work before a grant change begins.
// Callers cannot construct a PreparedEvent with forged, unredacted fields.
func (s *Service) Prepare(ctx context.Context, event Event) (PreparedEvent, error) {
	if s == nil || s.db == nil { return PreparedEvent{}, fmt.Errorf("audit service unavailable") }
	event.EventType = strings.TrimSpace(event.EventType)
	if event.EventType == "" || len(event.EventType) > 128 {
		return PreparedEvent{}, fmt.Errorf("audit event type is required")
	}
	if event.Result != ResultSuccess && event.Result != ResultRejected && event.Result != ResultFailed {
		return PreparedEvent{}, fmt.Errorf("invalid audit result")
	}

	metadata := json.RawMessage(`{}`)
	if event.Metadata != nil {
		raw, err := json.Marshal(event.Metadata)
		if err != nil { return PreparedEvent{}, fmt.Errorf("encode audit metadata: %w", err) }
		metadata = raw
	}
	// Secret redaction can query SQLite; ALWAYS complete it before entering
	// any writer transaction on StageCore's single-connection SQL pool.
	if s.secrets != nil {
		metadata = s.secrets.RedactJSON(ctx, metadata)
		event.Reason = s.secrets.RedactString(ctx, event.Reason)
	}
	auditID, err := stageid.New()
	if err != nil { return PreparedEvent{}, err }
	return PreparedEvent{record: Record{
		AuditID: auditID, EventType: event.EventType, OccurredAt: s.now().UTC(),
		ActorUserID: strings.TrimSpace(event.ActorUserID), ActorUsername: strings.TrimSpace(event.ActorUsername),
		Source: strings.TrimSpace(event.Source), ResourceType: strings.TrimSpace(event.ResourceType),
		ResourceID: strings.TrimSpace(event.ResourceID), Result: event.Result,
		Reason: strings.TrimSpace(event.Reason), CorrelationID: strings.TrimSpace(event.CorrelationID),
		Metadata: metadata,
	}}, nil
}

// Append retains the regular stand-alone auditing behavior.
func (s *Service) Append(ctx context.Context, event Event) (Record, error) {
	prepared, err := s.Prepare(ctx, event)
	if err != nil { return Record{}, err }
	return s.appendPrepared(ctx, s.db, prepared)
}

// AppendPreparedTx persists an already redacted event in an existing SQL
// transaction. Nothing here reads the DB or secret store independently.
func (s *Service) AppendPreparedTx(ctx context.Context, tx *sql.Tx, event PreparedEvent) (Record, error) {
	if s == nil || tx == nil { return Record{}, fmt.Errorf("audit transaction unavailable") }
	return s.appendPrepared(ctx, tx, event)
}

type auditExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Service) appendPrepared(ctx context.Context, executor auditExecutor, event PreparedEvent) (Record, error) {
	record := event.record
	if record.AuditID == "" || record.EventType == "" || len(record.Metadata) == 0 {
		return Record{}, fmt.Errorf("unprepared audit event")
	}
	_, err := executor.ExecContext(ctx, `
		INSERT INTO security_audit_records
		(audit_id, event_type, occurred_at_us, actor_user_id, actor_username, source,
		 resource_type, resource_id, result, reason, correlation_id, metadata_json)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.AuditID, record.EventType, record.OccurredAt.UnixMicro(), record.ActorUserID,
		record.ActorUsername, record.Source, record.ResourceType, record.ResourceID,
		record.Result, record.Reason, record.CorrelationID, string(record.Metadata))
	if err != nil { return Record{}, fmt.Errorf("append security audit: %w", err) }
	return record, nil
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
