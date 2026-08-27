package transfer

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-05-vault-transfer/internal/vault"
)

type Server struct {
	Vault      *vault.Store
	Gate       *Gate
	ChunkSize  int
	ChunkDelay time.Duration
	bytesSent  atomic.Int64
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /runtime/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "PONG")
	})
	mux.HandleFunc("GET /objects/{sha}", s.handleObject)
	return mux
}

func (s *Server) BytesSent() int64 { return s.bytesSent.Load() }

func (s *Server) handleObject(w http.ResponseWriter, r *http.Request) {
	sha := r.PathValue("sha")
	f, st, err := s.Vault.Open(sha)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	start, partial, err := parseRange(r.Header.Get("Range"), st.Size())
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", st.Size()))
		http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		http.Error(w, "seek failed", http.StatusInternalServerError)
		return
	}

	remaining := st.Size() - start
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("ETag", `"sha256:`+sha+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(remaining, 10))
	if partial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, st.Size()-1, st.Size()))
		w.WriteHeader(http.StatusPartialContent)
	}

	chunk := s.ChunkSize
	if chunk <= 0 {
		chunk = 256 * 1024
	}
	buf := make([]byte, chunk)
	for remaining > 0 {
		s.Gate.WaitBulkAllowed()
		nmax := int64(len(buf))
		if remaining < nmax {
			nmax = remaining
		}
		n, readErr := f.Read(buf[:nmax])
		if n > 0 {
			wn, writeErr := w.Write(buf[:n])
			if wn > 0 {
				s.bytesSent.Add(int64(wn))
				remaining -= int64(wn)
			}
			if writeErr != nil {
				return
			}
			if wn != n {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			if s.ChunkDelay > 0 {
				time.Sleep(s.ChunkDelay)
			}
		}
		if readErr == io.EOF {
			return
		}
		if readErr != nil {
			return
		}
	}
}

func parseRange(value string, size int64) (start int64, partial bool, err error) {
	if value == "" {
		return 0, false, nil
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return 0, false, fmt.Errorf("unsupported range")
	}
	spec := strings.TrimPrefix(value, "bytes=")
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 || parts[0] == "" {
		return 0, false, fmt.Errorf("only start-offset ranges are supported")
	}
	start, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, false, fmt.Errorf("invalid range")
	}
	return start, true, nil
}
