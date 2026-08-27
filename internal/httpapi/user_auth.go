package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const (
	browserSessionCookie   = "stagecore_session"
	csrfHeader             = "X-StageCore-CSRF"
	sessionRecheckInterval = 250 * time.Millisecond
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// WithUserAuth adds the authenticated local browser/API surface without
// changing Companion/device authentication. Browser credentials are accepted
// over TLS or explicit loopback development transport only.
func WithUserAuth(service *userauth.Service, hub *hubsecurity.Service) Option {
	return func(s *Server) {
		if service == nil || hub == nil {
			return
		}
		registerUserAuthRoutes(s.mux, service, hub)
	}
}

func registerUserAuthRoutes(mux *http.ServeMux, service *userauth.Service, hub *hubsecurity.Service) {
	mux.HandleFunc("GET /api/v1/auth/status", func(w http.ResponseWriter, r *http.Request) {
		if !secureBrowserRequest(w, r) {
			return
		}
		identity, err := hub.Identity(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "HUB_IDENTITY_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"hub_id": identity.HubID,
			"display_name": identity.DisplayName,
			"fingerprint": identity.Fingerprint,
			"bootstrap_state": identity.BootstrapState,
		})
	})

	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if !secureBrowserRequest(w, r) || !sameOriginRequest(w, r) {
			return
		}
		var body loginRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		credential, err := service.Login(r.Context(), body.Username, body.Password, loginRemoteKey(r))
		if err != nil {
			switch {
			case errors.Is(err, userauth.ErrLoginRateLimited):
				writeJSON(w, http.StatusTooManyRequests, map[string]any{"error_code": "LOGIN_RATE_LIMITED"})
			case errors.Is(err, userauth.ErrInvalidCredentials):
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error_code": "INVALID_CREDENTIALS"})
			default:
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "AUTH_UNAVAILABLE"})
			}
			return
		}
		setBrowserSessionCookie(w, r, credential.Token, credential.Session.ExpiresAt)
		writeJSON(w, http.StatusOK, map[string]any{
			"user": credential.Session.User,
			"expires_at": credential.Session.ExpiresAt.Format(time.RFC3339Nano),
			"csrf_token": credential.CSRFToken,
		})
	})

	mux.HandleFunc("GET /api/v1/auth/me", withPermission(service, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		writeJSON(w, http.StatusOK, map[string]any{
			"user": session.User,
			"expires_at": session.ExpiresAt.Format(time.RFC3339Nano),
		})
	}))

	mux.HandleFunc("POST /api/v1/auth/logout", withAuthenticatedSession(service, true, func(w http.ResponseWriter, r *http.Request, session userauth.Session, token string) {
		_ = session
		if err := service.Logout(r.Context(), token); err != nil && !errors.Is(err, userauth.ErrSessionInvalid) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "AUTH_UNAVAILABLE"})
			return
		}
		clearBrowserSessionCookie(w, r)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/v1/events", withPermission(service, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		serveAuthenticatedSSE(w, r, service, hub, session)
	}))
}

type sessionHandler func(http.ResponseWriter, *http.Request, userauth.Session)
type sessionTokenHandler func(http.ResponseWriter, *http.Request, userauth.Session, string)

func withPermission(service *userauth.Service, permission userauth.Permission, next sessionHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withAuthenticatedSession(service, isUnsafeMethod(r.Method), func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
			if err := userauth.Authorize(session.User.Role, permission); err != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
				return
			}
			next(w, r, session)
		})(w, r)
	}
}

func withAuthenticatedSession(service *userauth.Service, requireCSRF bool, next sessionTokenHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !secureBrowserRequest(w, r) || !sameOriginRequest(w, r) {
			return
		}
		token, ok := browserSessionToken(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error_code": "AUTH_REQUIRED"})
			return
		}
		var session userauth.Session
		var err error
		if requireCSRF {
			session, err = service.ValidateCSRF(r.Context(), token, r.Header.Get(csrfHeader))
		} else {
			session, err = service.Validate(r.Context(), token)
		}
		if err != nil {
			status := http.StatusUnauthorized
			code := "SESSION_INVALID"
			if requireCSRF {
				status = http.StatusForbidden
				code = "CSRF_INVALID"
			}
			writeJSON(w, status, map[string]any{"error_code": code})
			return
		}
		next(w, r, session, token)
	}
}

func secureBrowserRequest(w http.ResponseWriter, r *http.Request) bool {
	if secureDeviceRequest(r) {
		return true
	}
	writeJSON(w, http.StatusUpgradeRequired, map[string]any{"error_code": "SECURE_TRANSPORT_REQUIRED"})
	return false
}

func sameOriginRequest(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	expectedScheme := "http"
	if r.TLS != nil {
		expectedScheme = "https"
	}
	expected := expectedScheme + "://" + r.Host
	if origin != expected {
		writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "ORIGIN_FORBIDDEN"})
		return false
	}
	return true
}

func browserSessionToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(browserSessionCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return strings.TrimSpace(cookie.Value), true
}

func setBrowserSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: browserSessionCookie, Value: token, Path: "/",
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode,
		Expires: expires, MaxAge: int(time.Until(expires).Seconds()),
	})
}

func clearBrowserSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: browserSessionCookie, Value: "", Path: "/",
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode,
		Expires: time.Unix(1, 0), MaxAge: -1,
	})
}

func loginRemoteKey(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		// StageCore does not trust arbitrary forwarding as identity; this value is
		// only an additional throttle bucket. The direct peer remains included.
		return r.RemoteAddr + "|" + forwarded
	}
	return r.RemoteAddr
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func serveAuthenticatedSSE(w http.ResponseWriter, r *http.Request, service *userauth.Service, hub *hubsecurity.Service, session userauth.Session) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error_code": "REALTIME_UNAVAILABLE"})
		return
	}
	identity, err := hub.Identity(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "HUB_IDENTITY_UNAVAILABLE"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	payload, _ := json.Marshal(map[string]any{
		"type": "session.ready", "user": session.User,
		"hub_id": identity.HubID, "fingerprint": identity.Fingerprint,
	})
	_, _ = fmt.Fprintf(w, "event: session\ndata: %s\n\n", payload)
	flusher.Flush()

	token, ok := browserSessionToken(r)
	if !ok {
		return
	}
	ticker := time.NewTicker(sessionRecheckInterval)
	defer ticker.Stop()
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := service.Validate(r.Context(), token); err != nil {
				return
			}
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
