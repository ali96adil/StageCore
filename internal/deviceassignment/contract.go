// Package deviceassignment defines fail-closed v2 Stage Device project scoping.
//
// This package is a pure contract for the pending Hub-owned assignment
// protocol. It does not update the DB, send transport messages, authorize
// operators, or claim that a physical blackout has been independently verified.
package deviceassignment

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var (
	ErrInvalidAssignment = errors.New("invalid stage device assignment")
	ErrCommandScope      = errors.New("stage device command assignment mismatch")
	ErrInvalidBlackout   = errors.New("invalid stage device blackout acknowledgment")
)

type State string

const (
	Unassigned State = "UNASSIGNED"
	Preparing  State = "PREPARING"
	Active     State = "ACTIVE"
	Blocked    State = "BLOCKED"
)

// Assignment is a Hub-issued, device-scoped authority. Epoch starts at 1;
// the client never chooses or increments it.
type Assignment struct {
	DeviceID   string
	ProjectID  string
	Epoch      uint64
	State      State
	SnapshotID string
}

func (a Assignment) Validate() error {
	if strings.TrimSpace(a.DeviceID) == "" || a.DeviceID != strings.TrimSpace(a.DeviceID) || a.Epoch == 0 {
		return fmt.Errorf("%w: device identity and nonzero epoch required", ErrInvalidAssignment)
	}
	switch a.State {
	case Unassigned:
		if a.ProjectID != "" || a.SnapshotID != "" {
			return fmt.Errorf("%w: unassigned node cannot retain project or snapshot authority", ErrInvalidAssignment)
		}
	case Preparing, Blocked:
		if a.ProjectID != strings.TrimSpace(a.ProjectID) || a.SnapshotID != "" {
			return fmt.Errorf("%w: blocked/preparing state cannot carry a snapshot", ErrInvalidAssignment)
		}
	case Active:
		if a.ProjectID == "" || a.ProjectID != strings.TrimSpace(a.ProjectID) {
			return fmt.Errorf("%w: active node requires one project", ErrInvalidAssignment)
		}
		if a.SnapshotID != strings.TrimSpace(a.SnapshotID) {
			return fmt.Errorf("%w: malformed snapshot identity", ErrInvalidAssignment)
		}
	default:
		return fmt.Errorf("%w: unknown state %q", ErrInvalidAssignment, a.State)
	}
	return nil
}

// CommandScope is the already-authenticated, Hub-issued command context.
// Safety blackout uses its own bounded fail-safe path; it is not a show command.
type CommandScope struct {
	DeviceID                 string
	ProjectID                string
	Epoch                    uint64
	RuntimeSnapshotID        string
	RequirePublishedSnapshot bool
}

// AuthorizeCommand rejects all commands for unassigned, blocked or preparing
// nodes, even if a client or stale transport claims a matching project.
func (a Assignment) AuthorizeCommand(command CommandScope) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.State != Active || command.DeviceID != a.DeviceID ||
		command.ProjectID == "" || command.ProjectID != a.ProjectID ||
		command.Epoch != a.Epoch {
		return fmt.Errorf("%w: node is not active or device/project/epoch differs", ErrCommandScope)
	}
	if command.RequirePublishedSnapshot && (a.SnapshotID == "" || command.RuntimeSnapshotID == "") {
		return fmt.Errorf("%w: published snapshot required", ErrCommandScope)
	}
	if command.RuntimeSnapshotID != "" &&
		(a.SnapshotID == "" || command.RuntimeSnapshotID != a.SnapshotID) {
		return fmt.Errorf("%w: snapshot differs", ErrCommandScope)
	}
	return nil
}

// TransferIntent is created by an authorized Hub-side operation after
// permission/SHOW checks. Validation here does NOT grant permission or mutate
// the recorded assignment.
type TransferIntent struct {
	DeviceID      string
	FromProjectID string
	ToProjectID   string
	ExpectedEpoch uint64
	Challenge     string
}

type BlackoutAck struct {
	DeviceID      string
	Epoch         uint64
	Challenge     string
	Blackout      bool
	ChannelLevels []uint8
}

// VerifyBlackout checks a fresh same-generation software acknowledgment and
// returns a candidate assignment, always blocked (or unassigned). Persistence
// requires a separate CAS transaction and physical qualification remains
// independent of this software observation.
func (t TransferIntent) VerifyBlackout(current Assignment, ack BlackoutAck) (Assignment, error) {
	if err := current.Validate(); err != nil {
		return Assignment{}, err
	}
	if t.DeviceID == "" || t.DeviceID != strings.TrimSpace(t.DeviceID) ||
		t.FromProjectID != strings.TrimSpace(t.FromProjectID) ||
		t.ToProjectID != strings.TrimSpace(t.ToProjectID) ||
		t.ExpectedEpoch == 0 || t.ExpectedEpoch == math.MaxUint64 ||
		t.Challenge == "" || t.Challenge != strings.TrimSpace(t.Challenge) ||
		t.FromProjectID == t.ToProjectID ||
		current.DeviceID != t.DeviceID || current.Epoch != t.ExpectedEpoch ||
		current.ProjectID != t.FromProjectID {
		return Assignment{}, fmt.Errorf("%w: transfer precondition mismatch", ErrInvalidAssignment)
	}
	if current.State != Active && current.State != Blocked && current.State != Unassigned {
		return Assignment{}, fmt.Errorf("%w: current state does not permit transfer", ErrInvalidAssignment)
	}
	if ack.DeviceID != t.DeviceID || ack.Epoch != t.ExpectedEpoch ||
		ack.Challenge != t.Challenge || !ack.Blackout || len(ack.ChannelLevels) == 0 {
		return Assignment{}, fmt.Errorf("%w: identity, epoch, challenge or blackout state mismatch", ErrInvalidBlackout)
	}
	for _, level := range ack.ChannelLevels {
		if level != 0 {
			return Assignment{}, fmt.Errorf("%w: nonzero channel", ErrInvalidBlackout)
		}
	}
	next := Assignment{
		DeviceID:  t.DeviceID,
		ProjectID: t.ToProjectID,
		Epoch:     current.Epoch + 1,
		State:     Blocked,
	}
	if next.ProjectID == "" {
		next.State = Unassigned
	}
	return next, nil
}
