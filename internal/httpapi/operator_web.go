package httpapi

import (
	"net/http"

	"github.com/ali96adil/StageCore/internal/operatorweb"
)

// WithOperatorWeb serves the offline Operator UI from assets compiled into the
// Hub binary. The UI itself never bypasses the authenticated JSON API surface;
// it is only a same-origin client for those routes.
func WithOperatorWeb() Option {
	return func(s *Server) {
		s.mux.HandleFunc("GET /", serveOperatorAsset("index.html", "text/html; charset=utf-8", true))
		s.mux.HandleFunc("GET /app.css", serveOperatorAsset("app.css", "text/css; charset=utf-8", false))
		s.mux.HandleFunc("GET /guided-ux.css", serveOperatorAsset("guided-ux.css", "text/css; charset=utf-8", false))
		s.mux.HandleFunc("GET /localization.css", serveOperatorAsset("localization.css", "text/css; charset=utf-8", false))
		s.mux.HandleFunc("GET /theme.css", serveOperatorAsset("theme.css", "text/css; charset=utf-8", false))
		s.mux.HandleFunc("GET /app.js", serveOperatorAsset("app.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /bootstrap.js", serveOperatorAsset("bootstrap.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /preflight.js", serveOperatorAsset("preflight.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /memory.js", serveOperatorAsset("memory.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /security.js", serveOperatorAsset("security.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /configuration.js", serveOperatorAsset("configuration.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /show-lock.js", serveOperatorAsset("show-lock.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /guided-ux.js", serveOperatorAsset("guided-ux.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /localization.js", serveOperatorAsset("localization.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /theme.js", serveOperatorAsset("theme.js", "application/javascript; charset=utf-8", false))
	}
}

func serveOperatorAsset(name, contentType string, exactRoot bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if exactRoot && r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !secureBrowserRequest(w, r) {
			return
		}
		content, err := operatorweb.Read(name)
		if err != nil {
			http.Error(w, "operator web unavailable", http.StatusInternalServerError)
			return
		}
		setOperatorWebSecurityHeaders(w)
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}
}

func setOperatorWebSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
}
