package camerarelay

import (
	"context"
	"encoding/json"
	"mime"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"sync/atomic"
	"testing"
	"time"
)

// This fake camera deliberately closes a stream after one JPEG. The relay
// should reconnect through a *new* upstream without dropping a healthy viewer.
func TestRecoverFromMidstreamEOF(t *testing.T) {
	var upstreamRequests, inFlight, peak atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		index := upstreamRequests.Add(1)
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		mw := multipart.NewWriter(w)
		w.Header().Set("Content-Type", "multipart/x-mixed-replace;boundary="+mw.Boundary())
		w.WriteHeader(http.StatusOK)
		partHeaders := make(textproto.MIMEHeader)
		partHeaders.Set("Content-Type", "image/jpeg")
		emit := func(value byte) bool {
			part, err := mw.CreatePart(partHeaders)
			if err != nil {
				return false
			}
			if _, err = part.Write(testJPEG(value)); err != nil {
				return false
			}
			w.(http.Flusher).Flush()
			return true
		}
		if !emit(byte(index)) {
			return
		}
		if index == 1 {
			_ = mw.Close()
			return
		}
		ticker := time.NewTicker(15 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-req.Context().Done():
				return
			case <-ticker.C:
				if !emit(byte(index)) {
					return
				}
			}
		}
	}))
	defer source.Close()
	relay, err := New(Config{
		SourceURL: source.URL, ReconnectDelay: 100 * time.Millisecond,
		FrameTimeout: time.Second,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- relay.Run(ctx) }()
	defer func() {
		cancel()
		<-done
	}()
	eventually(t, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		return relay.received >= 3 && relay.connected
	})
	if upstreamRequests.Load() < 2 {
		t.Fatal("relay did not reconnect after upstream EOF")
	}
	if peak.Load() != 1 {
		t.Fatalf("overlapping source streams: %d", peak.Load())
	}
}

func TestViewerDisconnectReleasesSlot(t *testing.T) {
	var upstream, peak atomic.Int32
	source := fakeCamera(t, &upstream, &peak, false)
	defer source.Close()
	relay, err := New(Config{
		SourceURL: source.URL, MaxClients: 1, ReconnectDelay: 100 * time.Millisecond,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- relay.Run(ctx) }()
	server := httptest.NewServer(relay.Handler())
	defer func() {
		cancel()
		server.Close()
		<-done
	}()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(server.URL + "/api/v0/stream")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first viewer HTTP %d", resp.StatusCode)
	}
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := multipart.NewReader(resp.Body, params["boundary"]).NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(frame); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		return len(relay.subscribers) == 1
	})
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		return len(relay.subscribers) == 0
	})
	next, err := client.Get(server.URL + "/api/v0/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Body.Close()
	if next.StatusCode != http.StatusOK {
		t.Fatalf("new viewer HTTP %d after disconnect", next.StatusCode)
	}
}

// A stale upstream must be reported unhealthy even if the TCP socket
// hasn't yet returned a timeout.
func TestHealthReportsStaleStream(t *testing.T) {
	relay, err := New(Config{SourceURL: "http://127.0.0.1:1/"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	relay.mu.Lock()
	relay.connected = true
	relay.lastFrame = time.Now().Add(-30 * time.Second)
	relay.mu.Unlock()
	rr := httptest.NewRecorder()
	relay.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v0/health", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("stale stream health HTTP %d", rr.Code)
	}
	var result map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["state"] != "reconnecting" {
		t.Fatalf("stale stream health: %s", rr.Body.String())
	}
}
