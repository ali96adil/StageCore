package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type envelope struct {
	Type         string         `json:"type"`
	Version      int            `json:"version"`
	MessageID    string         `json:"message_id,omitempty"`
	Companion    string         `json:"companion_id,omitempty"`
	Role         string         `json:"role,omitempty"`
	SnapshotID   string         `json:"runtime_snapshot_id,omitempty"`
	Execution    string         `json:"execution_id,omitempty"`
	Capability   string         `json:"capability,omitempty"`
	Status       string         `json:"status,omitempty"`
	AckLevel     string         `json:"ack_level,omitempty"`
	ErrorCode    string         `json:"error_code,omitempty"`
	Parameters   map[string]any `json:"parameters,omitempty"`
	Output       map[string]any `json:"output,omitempty"`
	Capabilities []string       `json:"capabilities,omitempty"`
}

type testState struct {
	mu                sync.Mutex
	connections       int
	companionID       string
	exec1Completed    int
	exec2Completed    int
	duplicateRejected bool
	staleRejected     bool
	done              chan error
}

func main() {
	addr := "127.0.0.1:18083"
	if v := os.Getenv("STAGECORE_SPIKE_ADDR"); v != "" {
		addr = v
	}

	state := &testState{done: make(chan error, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc("/companion", func(w http.ResponseWriter, r *http.Request) {
		if err := state.handle(w, r); err != nil {
			log.Printf("connection failed: %v", err)
			select {
			case state.done <- err:
			default:
			}
		}
	})
	server := &http.Server{Addr: addr, Handler: mux}

	go func() {
		log.Printf("SPK-03 hub simulator listening on ws://%s/companion", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case state.done <- err:
			default:
			}
		}
	}()

	select {
	case err := <-state.done:
		if err != nil {
			log.Printf("SPK-03 FAILED: %v", err)
			os.Exit(1)
		}
		log.Printf("SPK-03 PASS: reconnect, duplicate protection, snapshot rejection, and second execution verified")
		_ = server.Close()
	case <-time.After(15 * time.Second):
		log.Printf("SPK-03 FAILED: timeout")
		os.Exit(1)
	}
}

func (s *testState) handle(w http.ResponseWriter, r *http.Request) error {
	conn, rw, err := upgrade(w, r)
	if err != nil {
		return err
	}
	defer conn.Close()

	s.mu.Lock()
	s.connections++
	n := s.connections
	s.mu.Unlock()

	hello, err := readJSON(rw.Reader)
	if err != nil {
		return fmt.Errorf("read hello: %w", err)
	}
	if hello.Type != "companion.hello" || hello.Companion == "" {
		return fmt.Errorf("unexpected hello: %+v", hello)
	}
	s.mu.Lock()
	if s.companionID == "" {
		s.companionID = hello.Companion
	}
	if s.companionID != hello.Companion {
		s.mu.Unlock()
		return fmt.Errorf("companion identity changed across reconnect: %s != %s", s.companionID, hello.Companion)
	}
	s.mu.Unlock()

	if err := writeJSON(conn, envelope{Type: "session.ready", Version: 1, Role: "VIDEO-MAIN", SnapshotID: "snap-1"}); err != nil {
		return err
	}

	switch n {
	case 1:
		if err := writeJSON(conn, execRequest("exec-1", "snap-1", "first")); err != nil {
			return err
		}
		res, err := readJSON(rw.Reader)
		if err != nil {
			return err
		}
		if res.Type != "execution.result" || res.Execution != "exec-1" || res.Status != "COMPLETED" {
			return fmt.Errorf("unexpected exec-1 result: %+v", res)
		}
		s.mu.Lock()
		s.exec1Completed++
		s.mu.Unlock()
		return nil

	case 2:
		if err := writeJSON(conn, execRequest("exec-1", "snap-1", "duplicate")); err != nil {
			return err
		}
		duplicate, err := readJSON(rw.Reader)
		if err != nil {
			return err
		}
		if duplicate.Execution != "exec-1" || duplicate.Status != "REJECTED" || duplicate.ErrorCode != "DUPLICATE_EXECUTION" {
			return fmt.Errorf("duplicate was not safely rejected: %+v", duplicate)
		}
		s.mu.Lock()
		s.duplicateRejected = true
		s.mu.Unlock()

		if err := writeJSON(conn, execRequest("exec-stale", "snap-old", "stale")); err != nil {
			return err
		}
		stale, err := readJSON(rw.Reader)
		if err != nil {
			return err
		}
		if stale.Execution != "exec-stale" || stale.Status != "REJECTED" || stale.ErrorCode != "SNAPSHOT_MISMATCH" {
			return fmt.Errorf("stale snapshot was not rejected: %+v", stale)
		}
		s.mu.Lock()
		s.staleRejected = true
		s.mu.Unlock()

		if err := writeJSON(conn, execRequest("exec-2", "snap-1", "second")); err != nil {
			return err
		}
		res, err := readJSON(rw.Reader)
		if err != nil {
			return err
		}
		if res.Execution != "exec-2" || res.Status != "COMPLETED" {
			return fmt.Errorf("unexpected exec-2 result: %+v", res)
		}
		s.mu.Lock()
		s.exec2Completed++
		pass := s.exec1Completed == 1 && s.exec2Completed == 1 && s.duplicateRejected && s.staleRejected
		s.mu.Unlock()
		if !pass {
			return fmt.Errorf("scenario counters invalid")
		}
		if err := writeJSON(conn, envelope{Type: "test.complete", Version: 1}); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
		select {
		case s.done <- nil:
		default:
		}
		return nil

	default:
		return fmt.Errorf("unexpected extra connection %d", n)
	}
}

func execRequest(id, snapshot, message string) envelope {
	return envelope{Type: "execution.request", Version: 1, Execution: id, SnapshotID: snapshot, Capability: "local.echo", Parameters: map[string]any{"message": message}}
}

func upgrade(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, nil, errors.New("missing websocket upgrade")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, nil, errors.New("missing websocket key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking unsupported")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, rw, nil
}

func readJSON(r *bufio.Reader) (envelope, error) {
	payload, opcode, err := readFrame(r)
	if err != nil {
		return envelope{}, err
	}
	if opcode == 0x8 {
		return envelope{}, io.EOF
	}
	if opcode != 0x1 {
		return envelope{}, fmt.Errorf("unexpected opcode %d", opcode)
	}
	var e envelope
	if err := json.Unmarshal(payload, &e); err != nil {
		return envelope{}, err
	}
	return e, nil
}

func writeJSON(conn net.Conn, e envelope) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return writeFrame(conn, 0x1, b)
}

func readFrame(r *bufio.Reader) ([]byte, byte, error) {
	h := make([]byte, 2)
	if _, err := io.ReadFull(r, h); err != nil {
		return nil, 0, err
	}
	opcode := h[0] & 0x0f
	masked := h[1]&0x80 != 0
	n := uint64(h[1] & 0x7f)
	switch n {
	case 126:
		b := make([]byte, 2)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil, 0, err
		}
		n = uint64(binary.BigEndian.Uint16(b))
	case 127:
		b := make([]byte, 8)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil, 0, err
		}
		n = binary.BigEndian.Uint64(b)
	}
	if n > 1<<20 {
		return nil, 0, fmt.Errorf("frame too large: %d", n)
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return nil, 0, err
		}
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return nil, 0, err
	}
	if masked {
		for i := range p {
			p[i] ^= mask[i%4]
		}
	}
	return p, opcode, nil
}

func writeFrame(w io.Writer, opcode byte, payload []byte) error {
	h := []byte{0x80 | opcode}
	n := len(payload)
	switch {
	case n < 126:
		h = append(h, byte(n))
	case n <= 65535:
		h = append(h, 126, 0, 0)
		binary.BigEndian.PutUint16(h[len(h)-2:], uint16(n))
	default:
		h = append(h, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(h[len(h)-8:], uint64(n))
	}
	if _, err := w.Write(h); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
