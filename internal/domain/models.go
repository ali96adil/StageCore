package domain

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrRevisionFrozen = errors.New("project revision is frozen")
	ErrInvalidInput   = errors.New("invalid input")
	ErrConflict       = errors.New("conflict")
)

type ProjectLifecycle string

const (
	ProjectActive   ProjectLifecycle = "ACTIVE"
	ProjectFinal    ProjectLifecycle = "FINAL"
	ProjectArchived ProjectLifecycle = "ARCHIVED"
)

type RevisionStatus string

const (
	RevisionDraft      RevisionStatus = "DRAFT"
	RevisionValidated  RevisionStatus = "VALIDATED"
	RevisionSuperseded RevisionStatus = "SUPERSEDED"
)

type PriorityClass string

const (
	PriorityP0 PriorityClass = "P0"
	PriorityP1 PriorityClass = "P1"
	PriorityP2 PriorityClass = "P2"
	PriorityP3 PriorityClass = "P3"
)

type Project struct {
	ID                    string
	Name                  string
	Description           string
	LifecycleState        ProjectLifecycle
	CurrentRevisionID     string
	DefaultVenueProfileID *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type ProjectRevision struct {
	ID               string
	ProjectID        string
	RevisionNumber   int64
	Status           RevisionStatus
	ParentRevisionID *string
	CreatedAt        time.Time
	CreatedBy        string
	ChangeNote       string
}

type Cue struct {
	ID              string
	RevisionID      string
	DisplayLabel    string
	Name            string
	OrderIndex      int
	CueType         string
	Criticality     string
	Enabled         bool
	ExecutionPolicy json.RawMessage
	NotesSummary    string
	Actions         []Action
}

type Action struct {
	ID            string
	CueID         string
	OrderIndex    int
	ExecutionMode string
	TargetRef     string
	CapabilityKey string
	Parameters    json.RawMessage
	TimeoutPolicy json.RawMessage
	ErrorPolicy   json.RawMessage
	PriorityClass PriorityClass
	Enabled       bool
}

type ProjectDeviceAlias struct {
	ID            string
	ProjectID     string
	LogicalName   string
	LogicalType   string
	TargetRef     string
	GroupName     string
	ProjectConfig json.RawMessage
}

type InputDefinition struct {
	ID          string
	RevisionID  string
	Name        string
	SourceRef   string
	EventType   string
	ValueSchema json.RawMessage
	Enabled     bool
}

type OutputDefinition struct {
	ID            string
	RevisionID    string
	Name          string
	TargetRef     string
	CapabilityKey string
	ValueSchema   json.RawMessage
	Criticality   string
}

type Route struct {
	ID                  string
	RevisionID          string
	Name                string
	InputID             string
	ConditionDefinition json.RawMessage
	TransformDefinition json.RawMessage
	DelayMS             *int64
	DebounceMS          *int64
	PriorityClass       PriorityClass
	ErrorPolicy         json.RawMessage
	Enabled             bool
	Actions             []RouteAction
}

type RouteAction struct {
	ID         string
	RouteID    string
	OrderIndex int
	OutputID   *string
	CueID      *string
	Parameters json.RawMessage
}
