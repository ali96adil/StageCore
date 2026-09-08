package deviceexperience

import (
	"database/sql"
	"fmt"
	"time"
)

type Repository struct {
	db  *sql.DB
	now func() time.Time
}

type Option func(*Repository)

func WithClock(now func() time.Time) Option {
	return func(r *Repository) {
		if now != nil {
			r.now = now
		}
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
