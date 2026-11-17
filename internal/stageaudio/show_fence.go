package stageaudio

import (
	"context"
	"errors"
	"time"
)

// ShowFence serializes Hub audio mutations with SHOW admission. Once a local
// mutation has an uncertain result, SHOW requires operator recovery of the
// sidecar and Hub rather than trusting a potentially out-of-order status read.
type ShowFence struct {
	token     chan struct{}
	uncertain bool
}

func NewShowFence() *ShowFence {
	return &ShowFence{token: make(chan struct{}, 1)}
}

func (f *ShowFence) Acquire(ctx context.Context) (func(), error) {
	if f == nil {
		return func() {}, ctx.Err()
	}
	select {
	case f.token <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-f.token
			return nil, err
		}
		return func() { <-f.token }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// MarkUncertain must be called while holding the fence.
func (f *ShowFence) MarkUncertain() {
	if f != nil {
		f.uncertain = true
	}
}

func (f *ShowFence) AcquireShow(ctx context.Context, call func(context.Context, Request) (LocalReply, error)) (func(), error) {
	release, err := f.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	if f.uncertain {
		release()
		return nil, errors.New("StageAudio command outcome is uncertain")
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	reply, err := call(readCtx, Request{Operation: "STATUS"})
	if err == nil {
		for _, name := range []string{DeckA, DeckB} {
			deck, exists := reply.Status.Decks[name]
			if !exists || deck.Commanded != "STOPPED" || deck.File != "" || deck.LastError != "" {
				err = errors.New("both StageAudio decks must report stopped before SHOW")
				break
			}
		}
	}
	if err != nil {
		release()
		return nil, err
	}
	return release, nil
}
