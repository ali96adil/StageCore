package stageaudio

import (
	"context"
	"errors"
	"testing"
)

func TestShowFenceRequiresConfirmedStoppedDecks(t *testing.T) {
	ctx := context.Background()
	fence := NewShowFence()
	stopped := func(_ context.Context, req Request) (LocalReply, error) {
		if req.Operation != "STATUS" {
			t.Fatal("SHOW admission issued a mutating command")
		}
		return LocalReply{Status: Status{Decks: map[string]DeckStatus{
			DeckA: {Commanded: "STOPPED"}, DeckB: {Commanded: "STOPPED"},
		}}}, nil
	}
	for _, call := range []func(context.Context, Request) (LocalReply, error){
		func(context.Context, Request) (LocalReply, error) { return LocalReply{}, errors.New("offline") },
		func(context.Context, Request) (LocalReply, error) { return LocalReply{}, nil },
		func(ctx context.Context, req Request) (LocalReply, error) {
			reply, _ := stopped(ctx, req)
			reply.Status.Decks[DeckA] = DeckStatus{Commanded: "PAUSED", File: "track.wav"}
			return reply, nil
		},
	} {
		if release, err := fence.AcquireShow(ctx, call); err == nil {
			release()
			t.Fatal("SHOW admitted without two confirmed stopped decks")
		}
	}
	release, err := fence.AcquireShow(ctx, stopped)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := fence.Acquire(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting mutation returned %v", err)
	}
	release()

	unlock, err := fence.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fence.MarkUncertain()
	unlock()
	if release, err := fence.AcquireShow(ctx, stopped); err == nil {
		release()
		t.Fatal("SHOW admitted after ambiguous sidecar mutation")
	}
}
