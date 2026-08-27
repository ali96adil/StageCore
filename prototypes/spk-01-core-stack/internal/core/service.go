package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrNoPublished     = errors.New("no published snapshot")
	ErrNoNextCue       = errors.New("no next cue")
)

type Persister interface {
	Load() (State, error)
	Save(State) error
}

type Service struct {
	mu    sync.RWMutex
	state State
	p     Persister
	bus   *Bus
	now   func() time.Time
}

func NewService(p Persister, bus *Bus) (*Service, error) {
	state, err := p.Load()
	if err != nil {
		return nil, err
	}
	if state.Projects == nil {
		state.Projects = map[string]*Project{}
	}
	return &Service{state: state, p: p, bus: bus, now: time.Now}, nil
}

func (s *Service) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
}

func (s *Service) CreateProject(name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, errors.New("project name is required")
	}
	id := newID("prj")
	var out Project
	err := s.commit(func(st *State) error {
		p := &Project{ID: id, Name: name}
		st.Projects[id] = p
		st.ActiveProjectID = id
		out = *p
		return nil
	})
	return out, err
}

func (s *Service) AddCue(projectID, number, name, message string) (Cue, error) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(name) == "" {
		return Cue{}, errors.New("project_id and cue name are required")
	}
	cue := Cue{ID: newID("cue"), Number: strings.TrimSpace(number), Name: strings.TrimSpace(name), Message: message}
	err := s.commit(func(st *State) error {
		p := st.Projects[projectID]
		if p == nil {
			return ErrProjectNotFound
		}
		p.DraftCues = append(p.DraftCues, cue)
		return nil
	})
	return cue, err
}

func (s *Service) Publish(projectID string) (Snapshot, error) {
	snap := Snapshot{ID: newID("snap"), CreatedAt: s.now().UTC()}
	err := s.commit(func(st *State) error {
		p := st.Projects[projectID]
		if p == nil {
			return ErrProjectNotFound
		}
		if len(p.DraftCues) == 0 {
			return errors.New("cannot publish project with no cues")
		}
		snap.Cues = append([]Cue(nil), p.DraftCues...)
		p.Published = &snap
		p.CurrentCueIndex = 0
		return nil
	})
	if err == nil {
		s.bus.Publish(s.event("snapshot.published", projectID, snap.ID, map[string]any{"cue_count": len(snap.Cues)}))
	}
	return snap, err
}

func (s *Service) Go(projectID string) (Execution, error) {
	var exec Execution
	var cue Cue
	err := s.commit(func(st *State) error {
		p := st.Projects[projectID]
		if p == nil {
			return ErrProjectNotFound
		}
		if p.Published == nil {
			return ErrNoPublished
		}
		if p.CurrentCueIndex >= len(p.Published.Cues) {
			return ErrNoNextCue
		}
		cue = p.Published.Cues[p.CurrentCueIndex]
		started := s.now().UTC()
		exec = Execution{ID: newID("exec"), CueID: cue.ID, CueName: cue.Name, SnapshotID: p.Published.ID, Status: "COMPLETED", Message: cue.Message, StartedAt: started, CompletedAt: started}
		p.Executions = append(p.Executions, exec)
		p.CurrentCueIndex++
		return nil
	})
	if err != nil {
		return Execution{}, err
	}

	s.bus.Publish(s.event("cue.started", projectID, exec.SnapshotID, map[string]any{"cue_id": cue.ID, "execution_id": exec.ID}))
	s.bus.Publish(s.event("action.completed", projectID, exec.SnapshotID, map[string]any{"cue_id": cue.ID, "execution_id": exec.ID, "adapter": "simulated"}))
	s.bus.Publish(s.event("cue.completed", projectID, exec.SnapshotID, map[string]any{"cue_id": cue.ID, "execution_id": exec.ID}))
	return exec, nil
}

func (s *Service) commit(mut func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneState(s.state)
	next.Version++
	if err := mut(&next); err != nil {
		return err
	}
	if err := s.p.Save(next); err != nil {
		return fmt.Errorf("persist state: %w", err)
	}
	s.state = next
	return nil
}

func (s *Service) event(kind, projectID, snapshotID string, payload map[string]any) Event {
	return Event{ID: newID("evt"), Type: kind, Occurred: s.now().UTC(), ProjectID: projectID, Snapshot: snapshotID, Payload: payload}
}

func cloneState(in State) State {
	b, _ := json.Marshal(in)
	var out State
	_ = json.Unmarshal(b, &out)
	if out.Projects == nil {
		out.Projects = map[string]*Project{}
	}
	return out
}

func newID(prefix string) string {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
