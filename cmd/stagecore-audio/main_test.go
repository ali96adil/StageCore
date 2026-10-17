package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/stageaudio"
)

type acceptingMPV struct{}
func (acceptingMPV) Send(_ context.Context, _ string, _ []any) error { return nil }

func newLocalController(t *testing.T) *stageaudio.Controller {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cue.wav"), []byte("fixture"), 0600); err != nil { t.Fatal(err) }
	c, err := stageaudio.NewController(root, acceptingMPV{})
	if err != nil { t.Fatal(err) }
	return c
}

func localRequest(t *testing.T, controller *stageaudio.Controller, message string) reply {
	t.Helper()
	client, server := net.Pipe()
	go handleConnection(context.Background(), server, controller)
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2*time.Second))
	if _, err := client.Write([]byte(message+"\n")); err != nil { t.Fatal(err) }
	var out reply
	if err := json.NewDecoder(bufio.NewReader(client)).Decode(&out); err != nil {
		t.Fatalf("local socket response stalled: %v", err)
	}
	return out
}

func TestLocalIPCIsLineFramedAndCanRespondWithoutClientClosing(t *testing.T) {
	controller := newLocalController(t)
	status := localRequest(t, controller, `{"operation":"STATUS"}`)
	if status.Error != "" || status.Status.Qualified || status.Status.Decks["A"].Commanded != "STOPPED" {
		t.Fatalf("unexpected idle local status: %+v", status)
	}
	load := localRequest(t, controller, `{"operation":"LOAD","deck":"A","file":"cue.wav"}`)
	if load.Error != "" || load.Status.Decks["A"].Commanded != "PAUSED" {
		t.Fatalf("unexpected load response: %+v", load)
	}
	play := localRequest(t, controller, `{"operation":"PLAY","deck":"A"}`)
	if play.Error != "" || play.Status.Decks["A"].Commanded != "PLAYING" {
		t.Fatalf("unexpected play response: %+v", play)
	}
}

func TestLocalIPCRejectsRawMPVInjectionAndOversize(t *testing.T) {
	controller := newLocalController(t)
	for _, req := range []string{
		`{"operation":"PLAY","deck":"A","command":["run","sh"]}`,
		`{"operation":"STOP","deck":"A"} {"operation":"PLAY","deck":"A"}`,
		`{"operation":"LOAD","deck":"A","file":"../other.wav"}`,
		`{"operation":"STATUS","junk":true}`,
		strings.Repeat("x", maxCommandBytes+1),
	} {
		out := localRequest(t, controller, req)
		if out.Error == "" {
			t.Errorf("malformed or unapproved IPC input accepted: %.70q", req)
		}
	}
}
