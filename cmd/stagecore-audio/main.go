// stagecore-audio is an opt-in, local-only rehearsal sound prototype.
// It never starts itself on boot and is not a SHOW-qualified player.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ali96adil/StageCore/internal/stageaudio"
)

const maxCommandBytes = 4096

type reply struct {
	Status stageaudio.Status `json:"status"`
	Error  string            `json:"error,omitempty"`
}

func main() {
	mediaRoot := flag.String("media-root", "", "existing directory of local rehearsal audio files (required)")
	runtimeRoot := flag.String("runtime-root", os.TempDir(), "parent directory for private local Unix sockets")
	flag.Parse()
	if *mediaRoot == "" {
		log.Fatal("--media-root is required; no audio service was started")
	}
	backend, err := stageaudio.NewMPVBackend(*runtimeRoot)
	if err != nil {
		log.Fatal(err)
	}
	defer backend.Close()
	controller, err := stageaudio.NewController(*mediaRoot, backend)
	if err != nil {
		log.Fatal(err)
	}
	socket := filepath.Join(backend.SocketDir(), "control.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0600); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("StageAudio LOCAL REHEARSAL PROTOTYPE: %s\n", socket)
	fmt.Println("SHOW UNQUALIFIED. No auto-play, network listener, or Hub Cue integration.")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("local audio IPC accept: %v", err)
			continue
		}
		go handleConnection(ctx, conn, controller)
	}
}

func handleConnection(ctx context.Context, conn net.Conn, controller *stageaudio.Controller) {
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return
	}
	// One typed JSON object per connection, with bounded request size and
	// strict field validation. Never forward caller-supplied raw mpv arrays.
	decoder := json.NewDecoder(io.LimitReader(conn, maxCommandBytes+1))
	decoder.DisallowUnknownFields()
	var request stageaudio.Request
	if err := decoder.Decode(&request); err != nil {
		_ = json.NewEncoder(conn).Encode(reply{Status: controller.Status(), Error: "INVALID_AUDIO_COMMAND"})
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		_ = json.NewEncoder(conn).Encode(reply{Status: controller.Status(), Error: "INVALID_AUDIO_COMMAND"})
		return
	}
	if request.Operation == "STATUS" {
		_ = json.NewEncoder(conn).Encode(reply{Status: controller.Status()})
		return
	}
	commandCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	status, err := controller.Execute(commandCtx, request)
	out := reply{Status: status}
	if err != nil {
		out.Error = "LOCAL_AUDIO_COMMAND_REJECTED"
	}
	_ = json.NewEncoder(conn).Encode(out)
}
