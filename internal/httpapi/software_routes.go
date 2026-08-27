package httpapi

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/software"
)

type softwarePackageJSON struct {
	PackageID          string `json:"package_id"`
	ProductID          string `json:"product_id"`
	Version            string `json:"version"`
	Platform           string `json:"platform"`
	Architecture       string `json:"architecture"`
	MinAPIVersion      int    `json:"min_api_version"`
	MaxAPIVersion      int    `json:"max_api_version"`
	SizeBytes          int64  `json:"size_bytes"`
	SHA256             string `json:"sha256"`
	SigningStatus      string `json:"signing_status"`
	NotarizationStatus string `json:"notarization_status"`
	ReleaseChannel     string `json:"release_channel"`
	ReleaseNotes       string `json:"release_notes"`
	Compatible         bool   `json:"compatible"`
	ProductionReady    bool   `json:"production_ready"`
	Preferred          bool   `json:"preferred"`
	CompatibilityNote  string `json:"compatibility_note"`
	DownloadPath       string `json:"download_path,omitempty"`
}

func (s *Server) handleSoftwarePackages(w http.ResponseWriter, r *http.Request) {
	statuses, err := s.software.List(
		r.Context(),
		strings.TrimSpace(r.URL.Query().Get("product_id")),
		strings.TrimSpace(r.URL.Query().Get("platform")),
		strings.TrimSpace(r.URL.Query().Get("architecture")),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error_code": "SOFTWARE_REPOSITORY_UNAVAILABLE"})
		return
	}
	packages := make([]softwarePackageJSON, 0, len(statuses))
	for _, status := range statuses {
		packages = append(packages, packageJSON(status))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hub_api_version": software.CurrentHubAPIVersion,
		"packages": packages,
	})
}

func (s *Server) handleSoftwareDownload(w http.ResponseWriter, r *http.Request) {
	packageID := strings.TrimSpace(r.PathValue("package_id"))
	file, status, err := s.software.OpenPackage(r.Context(), packageID)
	if errors.Is(err, domain.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "SOFTWARE_PACKAGE_NOT_FOUND"})
		return
	}
	if err != nil {
		if status.Package.ID != "" && !status.Compatible {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error_code": "SOFTWARE_PACKAGE_INCOMPATIBLE",
				"reason": status.Reason,
			})
			return
		}
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "SOFTWARE_PACKAGE_UNAVAILABLE"})
		return
	}
	defer file.Close()

	name := filepath.Base(strings.TrimSpace(status.Package.OriginalFilename))
	if name == "." || name == "" {
		name = status.Package.ProductID + "-" + status.Package.Version
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("ETag", `"sha256:`+status.Package.ContentHash+`"`)
	w.Header().Set("X-Content-SHA256", status.Package.ContentHash)
	w.Header().Set("X-Content-Length", int64String(status.Package.SizeBytes))
	http.ServeContent(w, r, name, status.Package.CreatedAt, file)
}

type softwareSetupRow struct {
	Version         string
	Architecture    string
	Channel         string
	Status          string
	Preferred       bool
	DownloadPath    string
	CanDownload     bool
	ProductionReady bool
}

func (s *Server) handleSoftwareSetup(w http.ResponseWriter, r *http.Request) {
	platform := strings.TrimSpace(r.URL.Query().Get("platform"))
	if platform == "" {
		platform = "macos"
	}
	architecture := strings.TrimSpace(r.URL.Query().Get("architecture"))
	statuses, err := s.software.List(r.Context(), "stagecore-companion", platform, architecture)
	if err != nil {
		http.Error(w, "software repository unavailable", http.StatusInternalServerError)
		return
	}
	rows := make([]softwareSetupRow, 0, len(statuses))
	for _, status := range statuses {
		rows = append(rows, softwareSetupRow{
			Version: status.Package.Version,
			Architecture: status.Package.Architecture,
			Channel: status.Package.ReleaseChannel,
			Status: status.Reason,
			Preferred: status.Preferred,
			DownloadPath: "/downloads/software/" + status.Package.ID,
			CanDownload: status.Compatible,
			ProductionReady: status.ProductionReady,
		})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := softwareSetupTemplate.Execute(w, map[string]any{
		"Platform": platform,
		"Architecture": architecture,
		"HubAPI": software.CurrentHubAPIVersion,
		"Rows": rows,
	}); err != nil {
		return
	}
}

func packageJSON(status software.PackageStatus) softwarePackageJSON {
	pkg := status.Package
	view := softwarePackageJSON{
		PackageID: pkg.ID, ProductID: pkg.ProductID, Version: pkg.Version,
		Platform: pkg.Platform, Architecture: pkg.Architecture,
		MinAPIVersion: pkg.MinAPIVersion, MaxAPIVersion: pkg.MaxAPIVersion,
		SizeBytes: pkg.SizeBytes, SHA256: pkg.ContentHash,
		SigningStatus: pkg.SigningStatus, NotarizationStatus: pkg.NotarizationStatus,
		ReleaseChannel: pkg.ReleaseChannel, ReleaseNotes: pkg.ReleaseNotes,
		Compatible: status.Compatible, ProductionReady: status.ProductionReady,
		Preferred: status.Preferred, CompatibilityNote: status.Reason,
	}
	if status.Compatible {
		view.DownloadPath = "/downloads/software/" + pkg.ID
	}
	return view
}

var softwareSetupTemplate = template.Must(template.New("software-setup").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>StageCore Downloads / Setup</title></head>
<body><main><h1>StageCore Downloads / Setup</h1>
<p>Local repository — Hub API v{{.HubAPI}}. Internet is not required.</p>
<h2>StageCore for {{.Platform}}</h2>
{{if .Architecture}}<p>Architecture: {{.Architecture}}</p>{{end}}
{{if .Rows}}<ul>{{range .Rows}}<li><strong>v{{.Version}}</strong> — {{.Architecture}} — {{.Channel}}
{{if .Preferred}} — PREFERRED{{end}}{{if .ProductionReady}} — PRODUCTION READY{{end}} — {{.Status}}
{{if .CanDownload}} — <a href="{{.DownloadPath}}">Download</a>{{else}} — download blocked{{end}}</li>{{end}}</ul>
{{else}}<p>No local packages available for this selection.</p>{{end}}
</main></body></html>`))
