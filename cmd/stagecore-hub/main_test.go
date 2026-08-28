package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeOSCInputRuntime struct {
	startSession string
	startListen  string
	localListen  string
	startErr     error
	serveErr     error
	serveCalled  chan struct{}
}

func (f *fakeOSCInputRuntime) StartOSCInput(_ context.Context, sessionID, listenAddress string) (string, error) {
	f.startSession = sessionID
	f.startListen = listenAddress
	return f.localListen, f.startErr
}

func (f *fakeOSCInputRuntime) ServeOSCInput(context.Context) error {
	close(f.serveCalled)
	return f.serveErr
}

func TestStartOSCInputWiresProductionRuntime(t *testing.T) {
	wantErr := errors.New("receiver stopped")
	runtime := &fakeOSCInputRuntime{
		localListen: "127.0.0.1:43123",
		serveErr:    wantErr,
		serveCalled: make(chan struct{}),
	}
	errCh := make(chan error, 1)

	listen, err := startOSCInput(context.Background(), runtime, "session-1", "127.0.0.1:9000", errCh)
	if err != nil {
		t.Fatal(err)
	}
	if listen != runtime.localListen || runtime.startSession != "session-1" || runtime.startListen != "127.0.0.1:9000" {
		t.Fatalf("listen=%q startSession=%q startListen=%q", listen, runtime.startSession, runtime.startListen)
	}

	select {
	case <-runtime.serveCalled:
	case <-time.After(time.Second):
		t.Fatal("ServeOSCInput was not called")
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, wantErr) {
			t.Fatalf("serve error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ServeOSCInput result was not reported")
	}
}

func TestStartOSCInputDoesNotServeAfterStartupFailure(t *testing.T) {
	wantErr := errors.New("startup failed")
	runtime := &fakeOSCInputRuntime{startErr: wantErr, serveCalled: make(chan struct{})}

	_, err := startOSCInput(context.Background(), runtime, "session-1", "127.0.0.1:9000", make(chan error, 1))
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v", err)
	}
	select {
	case <-runtime.serveCalled:
		t.Fatal("ServeOSCInput called after startup failure")
	default:
	}
}
