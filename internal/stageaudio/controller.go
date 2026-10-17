// Package stageaudio provides a two-deck, local-only rehearsal audio controller.
// It owns no browser endpoints and cannot dispatch StageCore Cues.
package stageaudio

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	DeckA = "A"
	DeckB = "B"
)

const maxMediaBytes int64 = 512 << 20

var (
	ErrInvalidCommand = errors.New("stageaudio: invalid command")
	ErrBackend        = errors.New("stageaudio: local player did not acknowledge command")
)

// Sender is intentionally a typed, internal transport; raw mpv commands must
// never be exposed as public/browser parameters.
type Sender interface {
	Send(context.Context, string, []any) error
}

type Request struct {
	Deck      string   `json:"deck,omitempty"`
	Operation string   `json:"operation"`
	File      string   `json:"file,omitempty"`
	Volume    *float64 `json:"volume,omitempty"`
	Loop      *bool    `json:"loop,omitempty"`
}

type DeckStatus struct {
	File          string  `json:"file,omitempty"`
	Commanded     string  `json:"commanded_state"`
	Volume        float64 `json:"volume"`
	Loop          bool    `json:"loop"`
	LastError     string  `json:"last_error,omitempty"`
}

type Status struct {
	MasterVolume float64               `json:"master_volume"`
	Decks        map[string]DeckStatus `json:"decks"`
	Qualified    bool                  `json:"show_qualified"`
	Note         string                `json:"note"`
}

type Controller struct {
	mu     sync.Mutex
	root   string
	sender Sender
	master float64
	decks  map[string]DeckStatus
}

func NewController(mediaRoot string, sender Sender) (*Controller, error) {
	if sender == nil || strings.TrimSpace(mediaRoot) == "" {
		return nil, fmt.Errorf("%w: media root and player are required", ErrInvalidCommand)
	}
	root, err := filepath.Abs(mediaRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve media root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: media root must be a directory", ErrInvalidCommand)
	}
	return &Controller{
		root: root, sender: sender, master: 70,
		decks: map[string]DeckStatus{
			DeckA: {Commanded: "STOPPED", Volume: 70},
			DeckB: {Commanded: "STOPPED", Volume: 70},
		},
	}, nil
}

func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statusLocked()
}

func (c *Controller) statusLocked() Status {
	copied := make(map[string]DeckStatus, 2)
	for id, deck := range c.decks {
		copied[id] = deck
	}
	return Status{
		MasterVolume: c.master,
		Decks: copied,
		Qualified: false,
		Note: "REHEARSAL_PROTOTYPE_LOCAL_ACK_ONLY_NOT_AUDIO_DELIVERY_PROOF",
	}
}

// Execute serializes commands across both decks. A local mpv IPC ACK means
// the command was accepted, not that a physical speaker emitted sound.
// A partially applied master-volume change is reported rather than hidden.
func (c *Controller) Execute(ctx context.Context, request Request) (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	op := strings.ToUpper(strings.TrimSpace(request.Operation))
	if op == "MASTER_VOLUME" {
		if !validVolume(request.Volume) {
			return c.statusLocked(), ErrInvalidCommand
		}
		newMaster := *request.Volume
		for _, name := range []string{DeckA, DeckB} {
			deck := c.decks[name]
			effective := deck.Volume * newMaster / 100
			if err := c.send(ctx, name, []any{"set_property", "volume", effective}); err != nil {
				deck.LastError = "MASTER_VOLUME_PARTIALLY_APPLIED_RECONCILIATION_REQUIRED"
				c.decks[name] = deck
				return c.statusLocked(), err
			}
		}
		c.master = newMaster
		for _, name := range []string{DeckA, DeckB} {
			deck := c.decks[name]
			deck.LastError = ""
			c.decks[name] = deck
		}
		return c.statusLocked(), nil
	}
	name := strings.ToUpper(strings.TrimSpace(request.Deck))
	deck, ok := c.decks[name]
	if !ok {
		return c.statusLocked(), ErrInvalidCommand
	}
	var command []any
	var updated DeckStatus = deck
	switch op {
	case "LOAD":
		path, err := c.safeMediaPath(request.File)
		if err != nil {
			return c.statusLocked(), err
		}
		// mpv launches with --pause=yes and never autoplays an old file.
		command = []any{"loadfile", path, "replace"}
		updated.File = filepath.Base(path)
		updated.Commanded = "PAUSED"
	case "PLAY":
		if deck.File == "" {
			return c.statusLocked(), fmt.Errorf("%w: load a file before playing", ErrInvalidCommand)
		}
		command = []any{"set_property", "pause", false}
		updated.Commanded = "PLAYING"
	case "PAUSE":
		if deck.File == "" {
			return c.statusLocked(), fmt.Errorf("%w: nothing loaded", ErrInvalidCommand)
		}
		command = []any{"set_property", "pause", true}
		updated.Commanded = "PAUSED"
	case "STOP":
		command = []any{"stop"}
		updated.File = ""
		updated.Commanded = "STOPPED"
	case "VOLUME":
		if !validVolume(request.Volume) {
			return c.statusLocked(), ErrInvalidCommand
		}
		command = []any{"set_property", "volume", *request.Volume * c.master / 100}
		updated.Volume = *request.Volume
	case "LOOP":
		if request.Loop == nil {
			return c.statusLocked(), ErrInvalidCommand
		}
		value := "no"
		if *request.Loop {
			value = "inf"
		}
		command = []any{"set_property", "loop-file", value}
		updated.Loop = *request.Loop
	default:
		return c.statusLocked(), fmt.Errorf("%w: unsupported operation", ErrInvalidCommand)
	}
	if err := c.send(ctx, name, command); err != nil {
		deck.LastError = "LOCAL_MPV_COMMAND_FAILED"
		c.decks[name] = deck
		return c.statusLocked(), err
	}
	updated.LastError = ""
	c.decks[name] = updated
	return c.statusLocked(), nil
}

func (c *Controller) send(ctx context.Context, deck string, command []any) error {
	if err := c.sender.Send(ctx, deck, command); err != nil {
		return fmt.Errorf("%w: %v", ErrBackend, err)
	}
	return nil
}

func validVolume(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) &&
		*value >= 0 && *value <= 100
}

func (c *Controller) safeMediaPath(name string) (string, error) {
	// Restrict the MVP to direct children of the configured media root.
	// No absolute paths, traversal, symlinks, directories or remote URLs.
	if name == "" || name != filepath.Base(name) ||
		strings.ContainsAny(name, "/\\:") || strings.Contains(name, "..") ||
		strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("%w: invalid local media name", ErrInvalidCommand)
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".wav", ".mp3", ".flac":
	default:
		return "", fmt.Errorf("%w: unsupported media file type", ErrInvalidCommand)
	}
	abs := filepath.Join(c.root, name)
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxMediaBytes {
		return "", fmt.Errorf("%w: unavailable or unsafe local media", ErrInvalidCommand)
	}
	return abs, nil
}
