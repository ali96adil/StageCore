package stageaudio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sentCommand struct {
	Deck string
	Args []any
}

type fakeSender struct {
	Calls []sentCommand
	FailDeck string
}

func (f *fakeSender) Send(_ context.Context, deck string, args []any) error {
	f.Calls = append(f.Calls, sentCommand{Deck: deck, Args: args})
	if f.FailDeck == deck {
		return errors.New("backend unavailable")
	}
	return nil
}

func controllerFixture(t *testing.T) (*Controller, *fakeSender, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wind.wav"), []byte("test-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "music.mp3"), []byte("test-data"), 0600); err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{}
	c, err := NewController(root, sender)
	if err != nil {
		t.Fatal(err)
	}
	return c, sender, root
}

func command(t *testing.T, controller *Controller, request Request) Status {
	t.Helper()
	result, err := controller.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("%+v: %v", request, err)
	}
	return result
}

func TestTwoDecksMixIndependentlyAndNeverAutoPlay(t *testing.T) {
	c, sender, _ := controllerFixture(t)
	a := command(t, c, Request{Deck: DeckA, Operation: "LOAD", File: "music.mp3"})
	if a.Decks[DeckA].Commanded != "PAUSED" || a.Decks[DeckB].Commanded != "STOPPED" {
		t.Fatalf("loading must not auto-play or change second deck: %+v", a)
	}
	command(t, c, Request{Deck: DeckA, Operation: "PLAY"})
	command(t, c, Request{Deck: DeckB, Operation: "LOAD", File: "wind.wav"})
	st := command(t, c, Request{Deck: DeckB, Operation: "PLAY"})
	if st.Decks[DeckA].Commanded != "PLAYING" || st.Decks[DeckB].Commanded != "PLAYING" {
		t.Fatalf("deck state collision: %+v", st.Decks)
	}
	if st.Qualified || !strings.Contains(st.Note, "NOT_AUDIO_DELIVERY_PROOF") {
		t.Fatalf("transport ACK must not claim physical qualification: %+v", st)
	}
	paused := sender.Calls[0]
	cmd := sender.Calls[1]
	if paused.Deck != DeckA || paused.Args[0] != "set_property" ||
		paused.Args[1] != "pause" || paused.Args[2] != true ||
		cmd.Deck != DeckA || cmd.Args[0] != "loadfile" || cmd.Args[2] != "replace" {
		t.Fatalf("LOAD must pause before swapping media: pause=%+v load=%+v", paused, cmd)
	}
	st = command(t, c, Request{Deck: DeckA, Operation: "STOP"})
	if st.Decks[DeckA].File != "" || st.Decks[DeckA].Commanded != "STOPPED" ||
		st.Decks[DeckB].Commanded != "PLAYING" {
		t.Fatalf("stop affected other deck or retained autoplay file: %+v", st)
	}
}

func TestDeckAndMasterVolumeClampedAndIndependent(t *testing.T) {
	c, sender, _ := controllerFixture(t)
	aVolume, bVolume, master := 42.0, 75.0, 50.0
	command(t, c, Request{Deck: DeckA, Operation: "VOLUME", Volume: &aVolume})
	command(t, c, Request{Deck: DeckB, Operation: "VOLUME", Volume: &bVolume})
	st := command(t, c, Request{Operation: "MASTER_VOLUME", Volume: &master})
	if st.MasterVolume != 50 || st.Decks[DeckA].Volume != 42 || st.Decks[DeckB].Volume != 75 {
		t.Fatalf("volume state: %+v", st)
	}
	l := len(sender.Calls)
	if sender.Calls[l-2].Deck != DeckA || sender.Calls[l-2].Args[2] != 21.0 ||
		sender.Calls[l-1].Deck != DeckB || sender.Calls[l-1].Args[2] != 37.5 {
		t.Fatalf("effective volume not scaled safely: %+v", sender.Calls[l-2:])
	}
	for _, invalid := range []float64{-1, 100.1} {
		if _, err := c.Execute(context.Background(), Request{Operation: "MASTER_VOLUME", Volume: &invalid}); err == nil {
			t.Fatalf("accepted invalid master volume %f", invalid)
		}
	}
}

func TestRejectTraversalSymlinksRemoteURLsAndUnsupportedFiles(t *testing.T) {
	c, sender, root := controllerFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.mp3")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil { t.Fatal(err) }
	if err := os.Symlink(outside, filepath.Join(root, "link.mp3")); err != nil { t.Fatal(err) }
	for _, name := range []string{
		"../outside.mp3", "/etc/passwd", "http://elsewhere/media.mp3",
		"subdir/a.mp3", "link.mp3", ".hidden.mp3", "music.exe",
		"missing.wav", "", "..music.mp3",
	} {
		if _, err := c.Execute(context.Background(), Request{Deck: DeckA, Operation: "LOAD", File: name}); err == nil {
			t.Errorf("unsafe local media accepted: %q", name)
		}
	}
	if len(sender.Calls) != 0 { t.Fatalf("unsafe input reached player: %+v", sender.Calls) }
}

func TestRejectInvalidCommandsAndBackendFailureDoesNotClaimSuccess(t *testing.T) {
	c, sender, _ := controllerFixture(t)
	for _, req := range []Request{
		{Deck: "C", Operation: "LOAD", File: "music.mp3"},
		{Deck: DeckA, Operation: "PLAY"},
		{Deck: DeckA, Operation: "PAUSE"},
		{Deck: DeckA, Operation: "SHELL", File: "music.mp3"},
		{Deck: DeckA, Operation: "VOLUME"},
		{Deck: DeckA, Operation: "LOOP"},
	} {
		if _, err := c.Execute(context.Background(), req); !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("%+v: expected invalid command, got %v", req, err)
		}
	}
	sender.FailDeck = DeckB
	result, err := c.Execute(context.Background(), Request{Deck: DeckB, Operation: "LOAD", File: "wind.wav"})
	if !errors.Is(err, ErrBackend) || result.Decks[DeckB].File != "" ||
		result.Decks[DeckB].Commanded != "STOPPED" || result.Decks[DeckB].LastError == "" {
		t.Fatalf("local failed load must not claim successful playback: status=%+v err=%v", result, err)
	}
}

func TestMasterVolumePartialUpdateIsExplicitlyDegraded(t *testing.T) {
	c, sender, _ := controllerFixture(t)
	newMaster := 90.0
	sender.FailDeck = DeckB
	st, err := c.Execute(context.Background(), Request{Operation: "MASTER_VOLUME", Volume: &newMaster})
	if !errors.Is(err, ErrBackend) || st.MasterVolume != 70 ||
		st.Decks[DeckB].LastError != "MASTER_VOLUME_PARTIALLY_APPLIED_RECONCILIATION_REQUIRED" {
		t.Fatalf("partial master change must be visible, status=%+v err=%v", st, err)
	}
}

func TestLoopAndPauseRemainDeckScoped(t *testing.T) {
	c, sender, _ := controllerFixture(t)
	command(t, c, Request{Deck: DeckA, Operation: "LOAD", File: "music.mp3"})
	enabled := true
	status := command(t, c, Request{Deck: DeckA, Operation: "LOOP", Loop: &enabled})
	if !status.Decks[DeckA].Loop || status.Decks[DeckB].Loop {
		t.Fatalf("loop leaked across decks: %+v", status)
	}
	if sender.Calls[len(sender.Calls)-1].Args[2] != "inf" {
		t.Fatal("mpv loop command must use fixed allowlisted value")
	}
}
