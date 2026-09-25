package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/discovery"
)

func TestRetryBonjourDiscoveryRecoversAfterTransientFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gateway := &deviceGateway{}
	announcement, err := discovery.NewAnnouncement(
		"01a045ef-1d7d-7b9b-8bb3-c0daa63fc19d",
		"StageCore Hub",
		"SHA256:hub",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"0.0.0.0:7841",
	)
	if err != nil {
		t.Fatal(err)
	}

	retry := make(chan time.Time, 2)
	var attempts atomic.Int32
	wantAdvertiser := &discovery.Advertiser{}
	starter := func(context.Context, discovery.Announcement) (*discovery.Advertiser, error) {
		if attempts.Add(1) == 1 {
			return nil, errors.New("network not ready")
		}
		return wantAdvertiser, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		retryBonjourDiscovery(
			ctx,
			logger,
			gateway,
			announcement,
			"hub-1",
			"0.0.0.0:7841",
			retry,
			starter,
		)
	}()

	retry <- time.Now()
	retry <- time.Now()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Bonjour retry did not recover")
	}

	if got := attempts.Load(); got != 2 {
		t.Fatalf("discovery attempts = %d, want 2", got)
	}
	gateway.advertiserMu.Lock()
	gotAdvertiser := gateway.advertiser
	gateway.advertiserMu.Unlock()
	if gotAdvertiser != wantAdvertiser {
		t.Fatal("recovered advertiser was not installed")
	}
}

func TestRetryBonjourDiscoveryStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gateway := &deviceGateway{}
	announcement, err := discovery.NewAnnouncement(
		"01a045ef-1d7d-7b9b-8bb3-c0daa63fc19d",
		"StageCore Hub",
		"SHA256:hub",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"0.0.0.0:7841",
	)
	if err != nil {
		t.Fatal(err)
	}

	retry := make(chan time.Time)
	var attempts atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		retryBonjourDiscovery(
			ctx,
			logger,
			gateway,
			announcement,
			"hub-1",
			"0.0.0.0:7841",
			retry,
			func(context.Context, discovery.Announcement) (*discovery.Advertiser, error) {
				attempts.Add(1)
				return nil, errors.New("unexpected attempt")
			},
		)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Bonjour retry did not stop after cancellation")
	}
	if got := attempts.Load(); got != 0 {
		t.Fatalf("discovery attempts after cancellation = %d, want 0", got)
	}
}
