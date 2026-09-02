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
		s.mux.HandleFunc("GET /workspace-profile.css", serveOperatorAsset("workspace-profile.css", "text/css; charset=utf-8", false))
		s.mux.HandleFunc("GET /first-run.css", serveOperatorAsset("first-run.css", "text/css; charset=utf-8", false))
		s.mux.HandleFunc("GET /extensions.css", serveOperatorAsset("extensions.css", "text/css; charset=utf-8", false))
		s.mux.HandleFunc("GET /app.js", serveOperatorAsset("app.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /bootstrap.js", serveOperatorAsset("bootstrap.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /preflight.js", serveOperatorAsset("preflight.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /memory.js", serveOperatorAsset("memory.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /security.js", serveOperatorAsset("security.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /configuration.js", serveOperatorAsset("configuration.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /show-lock.js", serveOperatorAsset("show-lock.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /guided-ux.js", serveOperatorAssetBundle([]string{"guided-ux.js", "device-profiles.js"}, "application/javascript; charset=utf-8"))
		s.mux.HandleFunc("GET /localization.js", serveOperatorAsset("localization.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /theme.js", serveOperatorAsset("theme.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /workspace-profile.js", serveOperatorAsset("workspace-profile.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /first-run.js", serveOperatorAsset("first-run.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /extensions.js", serveOperatorAsset("extensions.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /extensions-uninstall.js", serveOperatorAssetBundle([]string{"extensions-uninstall.js", "extensions-maintenance.js"}, "application/javascript; charset=utf-8"))
		s.mux.HandleFunc("GET /extensions-maintenance.js", serveOperatorAsset("extensions-maintenance.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /extensions-set-manifest.js", serveOperatorAsset("extensions-set-manifest.js", "application/javascript; charset=utf-8", false))
		s.mux.HandleFunc("GET /execution-environments.js", serveOperatorAssetBundle([]string{"execution-environments.js", "execution-environment-capture.js", "execution-environments-workspace.js"}, "application/javascript; charset=utf-8"))
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

func serveOperatorAssetBundle(names []string, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !secureBrowserRequest(w, r) {
			return
		}
		contents := make([][]byte, 0, len(names))
		for _, name := range names {
			content, err := operatorweb.Read(name)
			if err != nil {
				http.Error(w, "operator web unavailable", http.StatusInternalServerError)
				return
			}
			contents = append(contents, content)
		}
		setOperatorWebSecurityHeaders(w)
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		for _, content := range contents {
			_, _ = w.Write(content)
			_, _ = w.Write([]byte("\n"))
		}
	}
}

func setOperatorWebSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
}
