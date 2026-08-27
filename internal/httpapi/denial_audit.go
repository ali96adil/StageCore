package httpapi

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// AuditDeniedRequests records authenticated browser/API 403 responses without
// changing the authorization decision. The wrapper preserves streaming and
// WebSocket-related interfaces used elsewhere by the Hub.
func AuditDeniedRequests(next http.Handler, auth *userauth.Service, audit *securityaudit.Service) http.Handler {
	if next == nil || auth == nil || audit == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &denialAuditWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		if recorder.status != http.StatusForbidden {
			return
		}
		token, ok := browserSessionToken(r)
		if !ok {
			return
		}
		session, err := auth.Validate(r.Context(), token)
		if err != nil {
			return
		}
		_, _ = audit.Append(r.Context(), securityaudit.Event{
			EventType:     "authorization.denied",
			ActorUserID:   session.User.ID,
			ActorUsername: session.User.Username,
			Source:        loginRemoteKey(r),
			ResourceType:  "api",
			ResourceID:    r.Method + " " + r.URL.Path,
			Result:        securityaudit.ResultRejected,
			Reason:        "HTTP_403",
			Metadata: map[string]any{
				"method": r.Method,
				"path":   r.URL.Path,
			},
		})
	})
}

type denialAuditWriter struct {
	http.ResponseWriter
	status int
}

func (w *denialAuditWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *denialAuditWriter) Write(p []byte) (int, error) {
	return w.ResponseWriter.Write(p)
}

func (w *denialAuditWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *denialAuditWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *denialAuditWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *denialAuditWriter) ReadFrom(r io.Reader) (int64, error) {
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(r)
	}
	return io.Copy(w.ResponseWriter, r)
}
