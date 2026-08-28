package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeOSCInputRuntime struct {
	startProject string
	startListen  string
	localListen  string
	startErr     error
	serveErr     error
	serveCalled  chan struct{}
}

func (f *fakeOSCInputRuntime) StartOSCInputForProject(_ context.Context, projectID, listenAddress string) (string, error) {
	f.startProject = projectID
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

	listen, err := startOSCInput(context.Background(), runtime, "project-1", "127.0.0.1:9000", errCh)
	if err != nil {
		t.Fatal(err)
	}
	if listen != runtime.localListen || runtime.startProject != "project-1" || runtime.startListen != "127.0.0.1:9000" {
		t.Fatalf("listen=%q startProject=%q startListen=%q", listen, runtime.startProject, runtime.startListen)
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

	_, err := startOSCInput(context.Background(), runtime, "project-1", "127.0.0.1:9000", make(chan error, 1))
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v", err)
	}
	select {
	case <-runtime.serveCalled:
		t.Fatal("ServeOSCInput called after startup failure")
	default:
	}
}
