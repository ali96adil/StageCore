package deviceexperience

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
)

type EventRecorder interface {
	AppendEvent(context.Context, *string, contracts.EventEnvelope) (contracts.EventEnvelope, error)
}

type Repository struct {
	db     *sql.DB
	now    func() time.Time
	events EventRecorder
}

type Option func(*Repository)

func WithClock(now func() time.Time) Option {
	return func(r *Repository) {
		if now != nil {
			r.now = now
		}
	}
}

func WithEventRecorder(events EventRecorder) Option {
	return func(r *Repository) {
		r.events = events
	}
}

func NewRepository(db *sql.DB, options ...Option) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	r := &Repository{db: db, now: time.Now}
	for _, option := range options {
		option(r)
	}
	return r, nil
}
