package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ali96adil/StageCore/prototypes/spk-01-core-stack/internal/core"
)

type File struct {
	Path string
}

func (f File) Load() (core.State, error) {
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return core.State{Projects: map[string]*core.Project{}}, nil
	}
	if err != nil {
		return core.State{}, fmt.Errorf("read state: %w", err)
	}
	var st core.State
	if err := json.Unmarshal(b, &st); err != nil {
		return core.State{}, fmt.Errorf("decode state: %w", err)
	}
	return st, nil
}

func (f File) Save(st core.State) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), ".stagecore-state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, f.Path); err != nil {
		return err
	}
	return nil
}
