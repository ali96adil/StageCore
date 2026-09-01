package diagnosticsbundle

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/db"
	_ "modernc.org/sqlite"
)

var safeDeploymentKeys = map[string]struct{}{
	"STAGECORE_DATA_ROOT":                  {},
	"STAGECORE_VAULT_ROOT":                 {},
	"STAGECORE_LISTEN":                     {},
	"STAGECORE_DEVICE_LISTEN":              {},
	"STAGECORE_OSC_PLUGIN_PATH":            {},
	"STAGECORE_OSC_INPUT_LISTEN":           {},
	"STAGECORE_OSC_INPUT_PROJECT_ID":       {},
	"STAGECORE_RUNTIME_RESERVE_BYTES":      {},
	"STAGECORE_STORAGE_WARNING_PERCENT":    {},
}

type DeploymentMetadata struct {
	InstallRoot     string            `json:"install_root"`
	ConfigRoot      string            `json:"config_root"`
	SystemdUnit     string            `json:"systemd_unit"`
	EnvironmentPath string            `json:"environment_path"`
	ConfigReadable  bool              `json:"config_readable"`
	SafeValues      map[string]string `json:"safe_values,omitempty"`
	IgnoredKeyCount int               `json:"ignored_key_count"`
}

func collectDeploymentMetadata(opts Options) (DeploymentMetadata, string, error) {
	metadata := DeploymentMetadata{
		InstallRoot: opts.InstallRoot,
		ConfigRoot: opts.ConfigRoot,
		SystemdUnit: opts.SystemdUnit,
		EnvironmentPath: filepath.Join(opts.ConfigRoot, "stagecore.env"),
		SafeValues: map[string]string{},
	}
	content, err := os.ReadFile(metadata.EnvironmentPath)
	if err != nil {
		return metadata, "", fmt.Errorf("read managed deployment configuration: %w", err)
	}
	values, err := parseEnvironment(content)
	if err != nil {
		return metadata, "", fmt.Errorf("parse managed deployment configuration: %w", err)
	}
	metadata.ConfigReadable = true
	for key, value := range values {
		if _, allowed := safeDeploymentKeys[key]; allowed {
			metadata.SafeValues[key] = value
		} else {
			metadata.IgnoredKeyCount++
		}
	}
	return metadata, strings.TrimSpace(metadata.SafeValues["STAGECORE_DATA_ROOT"]), nil
}

func parseEnvironment(content []byte) (map[string]string, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid environment line")
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("empty environment key")
		}
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

type StateSummary struct {
	DatabaseSchema   int64              `json:"database_schema"`
	Projects         int                `json:"projects"`
	RuntimeSnapshots int                `json:"runtime_snapshots"`
	CueExecutions    int                `json:"cue_executions"`
	EventRecords     int                `json:"event_records"`
	LocalUsers       int                `json:"local_users"`
	Sessions         []SessionCount     `json:"sessions"`
	Companions       []CompanionCount   `json:"companions"`
	Extensions       []ExtensionSummary `json:"extensions"`
}

type SessionCount struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type CompanionCount struct {
	TrustState string `json:"trust_state"`
	Readiness  string `json:"readiness"`
	Count      int    `json:"count"`
}

type ExtensionSummary struct {
	ExtensionID   string `json:"extension_id"`
	Version       string `json:"version"`
	Kind          string `json:"kind"`
	Source        string `json:"source"`
	Lifecycle     string `json:"lifecycle"`
	DesiredState  string `json:"desired_state"`
	ObservedState string `json:"observed_state"`
	LastErrorCode string `json:"last_error_code,omitempty"`
}

func collectStateSummary(ctx context.Context, dataRoot string) (StateSummary, error) {
	dataRoot = strings.TrimSpace(dataRoot)
	if dataRoot == "" {
		return StateSummary{}, fmt.Errorf("data root is unavailable")
	}
	path := filepath.Join(filepath.Clean(dataRoot), "db", db.FileName)
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := url.Values{}
	q.Set("mode", "ro")
	q.Set("_busy_timeout", "1500")
	q.Add("_pragma", "query_only(ON)")
	u.RawQuery = q.Encode()
	database, err := sql.Open("sqlite", u.String())
	if err != nil {
		return StateSummary{}, fmt.Errorf("open StageCore database read-only: %w", err)
	}
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		return StateSummary{}, fmt.Errorf("read StageCore database: %w", err)
	}

	summary := StateSummary{}
	schema, err := db.SchemaVersion(database)
	if err != nil {
		return StateSummary{}, fmt.Errorf("read database schema: %w", err)
	}
	summary.DatabaseSchema = schema
	for query, target := range map[string]*int{
		`SELECT COUNT(*) FROM projects`:          &summary.Projects,
		`SELECT COUNT(*) FROM runtime_snapshots`: &summary.RuntimeSnapshots,
		`SELECT COUNT(*) FROM cue_executions`:    &summary.CueExecutions,
		`SELECT COUNT(*) FROM event_records`:     &summary.EventRecords,
		`SELECT COUNT(*) FROM local_users`:       &summary.LocalUsers,
	} {
		if err := database.QueryRowContext(ctx, query).Scan(target); err != nil {
			return StateSummary{}, fmt.Errorf("collect aggregate state count: %w", err)
		}
	}

	sessionRows, err := database.QueryContext(ctx, `
		SELECT session_type, status, COUNT(*)
		FROM sessions
		GROUP BY session_type, status
		ORDER BY session_type, status`)
	if err != nil {
		return StateSummary{}, fmt.Errorf("collect session summary: %w", err)
	}
	for sessionRows.Next() {
		var item SessionCount
		if err := sessionRows.Scan(&item.Type, &item.Status, &item.Count); err != nil {
			_ = sessionRows.Close()
			return StateSummary{}, fmt.Errorf("scan session summary: %w", err)
		}
		summary.Sessions = append(summary.Sessions, item)
	}
	if err := sessionRows.Err(); err != nil {
		_ = sessionRows.Close()
		return StateSummary{}, fmt.Errorf("iterate session summary: %w", err)
	}
	_ = sessionRows.Close()

	companionRows, err := database.QueryContext(ctx, `
		SELECT trust_state, readiness, COUNT(*)
		FROM companions
		GROUP BY trust_state, readiness
		ORDER BY trust_state, readiness`)
	if err != nil {
		return StateSummary{}, fmt.Errorf("collect companion summary: %w", err)
	}
	for companionRows.Next() {
		var item CompanionCount
		if err := companionRows.Scan(&item.TrustState, &item.Readiness, &item.Count); err != nil {
			_ = companionRows.Close()
			return StateSummary{}, fmt.Errorf("scan companion summary: %w", err)
		}
		summary.Companions = append(summary.Companions, item)
	}
	if err := companionRows.Err(); err != nil {
		_ = companionRows.Close()
		return StateSummary{}, fmt.Errorf("iterate companion summary: %w", err)
	}
	_ = companionRows.Close()

	extensionRows, err := database.QueryContext(ctx, `
		SELECT ep.extension_id, ep.version, ep.kind, ep.source, ei.lifecycle_state,
		       COALESCE(erl.desired_state, 'DISABLED'),
		       COALESCE(erl.observed_state, 'STOPPED'),
		       COALESCE(erl.last_error_code, '')
		FROM extension_installations ei
		JOIN extension_packages ep ON ep.package_id = ei.package_id
		LEFT JOIN extension_runtime_lifecycle erl ON erl.installation_id = ei.installation_id
		ORDER BY ep.extension_id, ep.version`)
	if err != nil {
		return StateSummary{}, fmt.Errorf("collect extension summary: %w", err)
	}
	for extensionRows.Next() {
		var item ExtensionSummary
		if err := extensionRows.Scan(
			&item.ExtensionID, &item.Version, &item.Kind, &item.Source, &item.Lifecycle,
			&item.DesiredState, &item.ObservedState, &item.LastErrorCode,
		); err != nil {
			_ = extensionRows.Close()
			return StateSummary{}, fmt.Errorf("scan extension summary: %w", err)
		}
		summary.Extensions = append(summary.Extensions, item)
	}
	if err := extensionRows.Err(); err != nil {
		_ = extensionRows.Close()
		return StateSummary{}, fmt.Errorf("iterate extension summary: %w", err)
	}
	_ = extensionRows.Close()

	if summary.Sessions == nil {
		summary.Sessions = []SessionCount{}
	}
	if summary.Companions == nil {
		summary.Companions = []CompanionCount{}
	}
	if summary.Extensions == nil {
		summary.Extensions = []ExtensionSummary{}
	}
	return summary, nil
}

func sortedSafeDeploymentKeys() []string {
	keys := make([]string, 0, len(safeDeploymentKeys))
	for key := range safeDeploymentKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
