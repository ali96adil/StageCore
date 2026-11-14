package stageaudio

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestMPVBackendHelperProcess is re-executed by a tiny fake mpv launcher. It
// exposes only Unix sockets under the test's temporary directory and accepts
// only the JSON IPC used by MPVBackend.
func TestMPVBackendHelperProcess(t *testing.T) {
	if os.Getenv("STAGECORE_TEST_MPV_HELPER") != "1" {
		return
	}
	socket := os.Getenv("STAGECORE_TEST_MPV_SOCKET")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen on fake mpv socket: %v", err)
	}
	defer listener.Close()
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		go serveFakeMPVIPC(t, socket, connection)
	}
}

func serveFakeMPVIPC(t *testing.T, socket string, connection net.Conn) {
	defer connection.Close()
	var request struct {
		Command   []any  `json:"command"`
		RequestID int64  `json:"request_id"`
		Deck      string `json:"deck"`
	}
	if err := json.NewDecoder(bufio.NewReader(connection)).Decode(&request); err != nil {
		return // waitForMPV probes socket readiness and closes without a request.
	}
	if logPath := os.Getenv("STAGECORE_TEST_MPV_COMMAND_LOG"); logPath != "" {
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Errorf("open fake mpv command log: %v", err)
			return
		}
		request.Deck = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(socket), "mpv-"), ".sock")
		if err := json.NewEncoder(file).Encode(request); err != nil {
			t.Errorf("record fake mpv command: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Errorf("close fake mpv command log: %v", err)
			return
		}
	}
	response := map[string]any{"request_id": request.RequestID, "error": "success", "data": map[string]any{"ok": true}}
	if len(request.Command) > 0 && request.Command[0] == "test_error" {
		response["error"] = "invalid parameter"
	}
	if err := json.NewEncoder(connection).Encode(response); err != nil {
		t.Errorf("write fake mpv response: %v", err)
	}
}

func TestMPVBackendOwnsTwoPrivateProcessesAndCleansUp(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("fake mpv integration requires Unix-domain sockets and a POSIX launcher")
	}
	root := shortMPVTestRoot(t)
	runtimeRoot := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeRoot, 0700); err != nil {
		t.Fatal(err)
	}
	commandLog := filepath.Join(root, "commands.jsonl")
	installFakeMPV(t, root, commandLog, "")
	backend, err := NewMPVBackend(runtimeRoot)
	if err != nil {
		t.Fatalf("start fake two-deck backend: %v\nhelper output: %s", err, readFakeMPVOutput(t, root))
	}
	dir := backend.SocketDir()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("private socket directory: %v", err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("private socket directory mode=%04o, want 0700", info.Mode().Perm())
	}
	if len(backend.players) != 2 || len(backend.waitDone) != 2 ||
		backend.sockets[DeckA] == "" || backend.sockets[DeckB] == "" || backend.sockets[DeckA] == backend.sockets[DeckB] {
		t.Fatalf("backend did not start isolated deck processes/sockets: players=%d sockets=%v", len(backend.players), backend.sockets)
	}
	for _, deck := range []string{DeckA, DeckB} {
		socketInfo, err := os.Stat(backend.sockets[deck])
		if err != nil || socketInfo.Mode()&os.ModeSocket == 0 {
			t.Fatalf("deck %s IPC is not a local Unix socket: info=%v err=%v", deck, socketInfo, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, deck := range []string{DeckA, DeckB} {
		data, err := backend.Query(ctx, deck, []any{"get_property", "pause"})
		if err != nil || string(data) != `{"ok":true}` {
			t.Fatalf("deck %s local IPC response=%s err=%v", deck, data, err)
		}
	}
	if _, err := backend.Query(ctx, "unknown", []any{"get_property", "pause"}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("unknown deck Query error=%v", err)
	}
	if _, err := backend.Query(ctx, DeckA, []any{"test_error"}); err == nil || !strings.Contains(err.Error(), "invalid parameter") {
		t.Fatalf("mpv command rejection was not surfaced: %v", err)
	}
	backend.Close()
	backend.Close() // Close is safe to repeat.
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private socket directory remained after Close: %v", err)
	}
	for i, cmd := range backend.players {
		select {
		case <-backend.waitDone[i]:
		default:
			t.Fatalf("fake mpv child %d was not reaped by Close", i)
		}
		if cmd.ProcessState == nil {
			t.Fatalf("fake mpv child %d has no waited process state", i)
		}
	}

	file, err := os.Open(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var requests []struct {
		Command   []any  `json:"command"`
		RequestID int64  `json:"request_id"`
		Deck      string `json:"deck"`
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var request struct {
			Command   []any  `json:"command"`
			RequestID int64  `json:"request_id"`
			Deck      string `json:"deck"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 || requests[0].RequestID == requests[1].RequestID ||
		requests[0].Deck != DeckA || requests[1].Deck != DeckB || requests[2].Deck != DeckA ||
		requests[2].Command[0] != "test_error" {
		t.Fatalf("fake mpv observed unexpected IPC requests: %+v", requests)
	}
}

func TestMPVBackendRemovesFirstDeckWhenSecondDeckFailsToStart(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("fake mpv integration requires Unix-domain sockets and a POSIX launcher")
	}
	root := shortMPVTestRoot(t)
	runtimeRoot := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeRoot, 0700); err != nil {
		t.Fatal(err)
	}
	installFakeMPV(t, root, filepath.Join(root, "commands.jsonl"), DeckB)
	if _, err := NewMPVBackend(runtimeRoot); err == nil || !strings.Contains(err.Error(), "deck B unavailable") {
		t.Fatalf("second deck startup failure was not surfaced: %v\nhelper output: %s", err, readFakeMPVOutput(t, root))
	}
	entries, err := os.ReadDir(runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed two-deck startup left a private runtime directory behind: %v", entries)
	}
}

func installFakeMPV(t *testing.T, root, commandLog, failDeck string) {
	t.Helper()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	launcher := `#!/bin/sh
for arg do
  case "$arg" in
    --input-ipc-server=*)
      STAGECORE_TEST_MPV_SOCKET=${arg#--input-ipc-server=}
      export STAGECORE_TEST_MPV_SOCKET
      case "$STAGECORE_TEST_MPV_FAIL_DECK:$STAGECORE_TEST_MPV_SOCKET" in
        B:*mpv-B.sock) exit 17 ;;
      esac
      ;;
  esac
done
export STAGECORE_TEST_MPV_HELPER=1
exec "$STAGECORE_TEST_EXECUTABLE" -test.run='^TestMPVBackendHelperProcess$' >>"$STAGECORE_TEST_MPV_STDOUT_LOG" 2>&1
`
	path := filepath.Join(bin, "mpv")
	if err := os.WriteFile(path, []byte(launcher), 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STAGECORE_TEST_EXECUTABLE", executable)
	t.Setenv("STAGECORE_TEST_MPV_COMMAND_LOG", commandLog)
	t.Setenv("STAGECORE_TEST_MPV_STDOUT_LOG", filepath.Join(root, "children.log"))
	t.Setenv("STAGECORE_TEST_MPV_FAIL_DECK", failDeck)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Fatalf("fake mpv launcher is not first on PATH: %v", err)
	}
}

func shortMPVTestRoot(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "sc-audio-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove fake MPV test root: %v", err)
		}
	})
	return root
}

func readFakeMPVOutput(t *testing.T, root string) string {
	t.Helper()
	output, err := os.ReadFile(filepath.Join(root, "children.log"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read fake mpv output: %v", err)
	}
	return string(output)
}
