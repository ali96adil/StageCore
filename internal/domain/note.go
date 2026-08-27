package domain

import "time"

type NoteStatus string

const (
	NoteOpen     NoteStatus = "OPEN"
	NoteResolved NoteStatus = "RESOLVED"
)

type Note struct {
	ID         string
	ProjectID  string
	SessionID  *string
	CueID      *string
	Category   string
	Body       string
	Status     NoteStatus
	CreatedBy  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ResolvedAt *time.Time
}
