// stagecore-audio is an opt-in, local-only rehearsal sound prototype.
// It never starts itself on boot and is not a SHOW-qualified player.
package main

import (
	"bufio"
	"bytes"
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
	"strings"
	"syscall"
	"time"

	"github.com/ali96adil/StageCore/internal/stageaudio"
)

const maxCommandBytes = 4096

type reply struct {
	Status stageaudio.Status `json:"status"`
	Outputs []stageaudio.AudioOutput `json:"outputs,omitempty"`
	Media []string `json:"media,omitempty"`
	BluetoothDevices []stageaudio.BluetoothDevice `json:"bluetooth_devices,omitempty"`
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
		_ = backend.Close()
		log.Fatal(err)
	}
	socket := filepath.Join(backend.SocketDir(), "control.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		_ = backend.Close()
		log.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0600); err != nil {
		_ = backend.Close()
		log.Fatal(err)
	}
	fmt.Printf("StageAudio LOCAL REHEARSAL PROTOTYPE: %s\n", socket)
	fmt.Println("SHOW UNQUALIFIED. No auto-play, network listener, or Hub Cue integration.")
	bluez := stageaudio.NewBluetoothService()
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
		go handleConnectionWithBluetooth(ctx, conn, controller, bluez)
	}
}

func handleConnection(ctx context.Context, conn net.Conn, controller *stageaudio.Controller) {
	handleConnectionWithBluetooth(ctx, conn, controller, nil)
}

type localBluetooth interface {
	PairedSpeakers(context.Context) ([]stageaudio.BluetoothDevice, error)
	ConnectPairedSpeaker(context.Context, string) (stageaudio.BluetoothDevice, error)
}

func handleConnectionWithBluetooth(ctx context.Context, conn net.Conn, controller *stageaudio.Controller, bluetooth localBluetooth) {
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return
	}
	// One typed JSON object per connection, with bounded request size and
	// strict field validation. Never forward caller-supplied raw mpv arrays.
	line, readErr := bufio.NewReader(io.LimitReader(conn, maxCommandBytes+2)).ReadBytes('\n')
	if readErr != nil || len(line) > maxCommandBytes {
		_ = json.NewEncoder(conn).Encode(reply{Status: controller.Status(), Error: "INVALID_AUDIO_COMMAND"})
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
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
	// The first request is complete; do not time out a short, opt-in BlueZ
	// connect simply because the client waited over 5 seconds.
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Now().Add(28 * time.Second))
	op := strings.ToUpper(strings.TrimSpace(request.Operation))
	if op == "STATUS" {
		_ = json.NewEncoder(conn).Encode(reply{Status: controller.Status()})
		return
	}
	if op == "FILES" {
		files, err := controller.MediaFiles()
		out := reply{Status: controller.Status(), Media: files}
		if err != nil { out.Error = "LOCAL_AUDIO_MEDIA_UNAVAILABLE" }
		_ = json.NewEncoder(conn).Encode(out)
		return
	}
	if op == "OUTPUTS" {
		qctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		outputs, err := controller.Outputs(qctx)
		out := reply{Status: controller.Status(), Outputs: outputs}
		if err != nil { out.Error = "LOCAL_OUTPUT_DISCOVERY_UNAVAILABLE" }
		_ = json.NewEncoder(conn).Encode(out)
		return
	}
	if op == "SELECT_OUTPUT" {
		qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		status, err := controller.SelectOutput(qctx, request.OutputID)
		out := reply{Status: status}
		if err != nil { out.Error = "LOCAL_OUTPUT_ROUTE_REJECTED" }
		_ = json.NewEncoder(conn).Encode(out)
		return
	}
	if op == "BT_DEVICES" || op == "BT_CONNECT" {
		out := reply{Status: controller.Status()}
		if bluetooth == nil {
			out.Error = "BLUETOOTH_BACKEND_UNAVAILABLE"
		} else if op == "BT_DEVICES" {
			qctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			devices, err := bluetooth.PairedSpeakers(qctx)
			if err != nil { out.Error = "BLUETOOTH_DISCOVERY_UNAVAILABLE" } else { out.BluetoothDevices = devices }
		} else if !controller.OutputChangeAllowed() {
			out.Error = "STOP_AUDIO_BEFORE_BLUETOOTH_CONNECT"
		} else {
			qctx, cancel := context.WithTimeout(ctx, 23*time.Second)
			defer cancel()
			device, err := bluetooth.ConnectPairedSpeaker(qctx, request.Address)
			if err != nil { out.Error = "BLUETOOTH_CONNECT_FAILED" } else { out.BluetoothDevices = []stageaudio.BluetoothDevice{device} }
		}
		_ = json.NewEncoder(conn).Encode(out)
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
