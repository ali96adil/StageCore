package doctor

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deployment"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	_ "modernc.org/sqlite"
)

const ReportSchemaVersion = 1

type Status string

const (
	Ready    Status = "READY"
	Warning  Status = "WARNING"
	Advisory Status = "ADVISORY"
	Blocker  Status = "BLOCKER"
)

type Overall string

const (
	OverallReady   Overall = "READY"
	OverallWarning Overall = "WARNING"
	OverallBlocked Overall = "BLOCKED"
)

type Check struct {
	ID          string   `json:"id"`
	Status      Status   `json:"status"`
	MessageKey  string   `json:"message_key"`
	MessageArgs []string `json:"message_args,omitempty"`
	Detail      string   `json:"detail,omitempty"`
	RemedyKey   string   `json:"remedy_key,omitempty"`
	RemedyArgs  []string `json:"remedy_args,omitempty"`
}

type Counts struct {
	Ready    int `json:"ready"`
	Warning  int `json:"warning"`
	Advisory int `json:"advisory"`
	Blocker  int `json:"blocker"`
}

type Report struct {
	SchemaVersion int       `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Overall       Overall   `json:"overall"`
	Counts        Counts    `json:"counts"`
	Checks        []Check   `json:"checks"`
}

type Options struct {
	InstallRoot string
	ConfigRoot  string
	SystemdUnit string
	HTTPTimeout time.Duration
}

type Filesystem struct {
	TotalBytes uint64
	FreeBytes  uint64
}

type CommandFunc func(context.Context, string, ...string) ([]byte, error)
type StatFSFunc func(string) (Filesystem, error)

type Runner struct {
	Command    CommandFunc
	HTTPClient *http.Client
	StatFS     StatFSFunc
	Now        func() time.Time
}

type deploymentConfig struct {
	DataRoot   string
	VaultRoot  string
	Listen     string
	PluginPath string
}

func DefaultOptions() Options {
	return Options{
		InstallRoot: deployment.DefaultInstallRoot,
		ConfigRoot:  deployment.DefaultConfigRoot,
		SystemdUnit: deployment.DefaultUnitPath,
		HTTPTimeout: 2 * time.Second,
	}
}

func NewRunner() *Runner {
	return &Runner{
		Command:    execCommand,
		HTTPClient: &http.Client{},
		StatFS:     statFilesystem,
		Now:        time.Now,
	}
}

func (r *Runner) Run(ctx context.Context, opts Options) Report {
	if r == nil {
		r = NewRunner()
	}
	r.normalize()
	opts = normalizeOptions(opts)

	report := Report{SchemaVersion: ReportSchemaVersion, GeneratedAt: r.Now().UTC()}

	cfg, configCheck := checkDeploymentConfig(opts)
	report.Checks = append(report.Checks, configCheck)
	report.Checks = append(report.Checks, checkBinaries(opts.InstallRoot))

	unitOK, unitCheck := checkSystemdUnit(opts)
	report.Checks = append(report.Checks, unitCheck)
	if unitOK {
		report.Checks = append(report.Checks, r.checkSystemdEnabled(ctx), r.checkSystemdActive(ctx))
	} else {
		report.Checks = append(report.Checks,
			skippedCheck("systemd.enabled", "check.skipped.unit", "remedy.systemd.unit"),
			skippedCheck("systemd.active", "check.skipped.unit", "remedy.systemd.unit"),
		)
	}

	if cfg != nil {
		report.Checks = append(report.Checks,
			r.checkStorage("storage.data", cfg.DataRoot),
			r.checkStorage("storage.vault", cfg.VaultRoot),
		)

		database, databaseCheck := inspectDatabase(ctx, cfg.DataRoot)
		report.Checks = append(report.Checks, databaseCheck)
		if database != nil {
			report.Checks = append(report.Checks, inspectPairing(ctx, database))
			_ = database.Close()
		} else {
			report.Checks = append(report.Checks, skippedCheck("pairing.summary", "check.skipped.database", "remedy.database"))
		}

		liveOK, liveCheck := r.checkHubLive(ctx, cfg.Listen, opts.HTTPTimeout)
		report.Checks = append(report.Checks, liveCheck)
		if liveOK {
			report.Checks = append(report.Checks, r.checkHubReady(ctx, cfg.Listen, opts.HTTPTimeout))
		} else {
			report.Checks = append(report.Checks, skippedCheck("hub.ready", "check.skipped.hub_live", "remedy.hub.active"))
		}
	} else {
		for _, id := range []string{"storage.data", "storage.vault", "database.readonly", "pairing.summary", "hub.live", "hub.ready"} {
			report.Checks = append(report.Checks, skippedCheck(id, "check.skipped.config", "remedy.config"))
		}
	}

	finalizeReport(&report)
	return report
}

func (r *Runner) normalize() {
	if r.Command == nil {
		r.Command = execCommand
	}
	if r.HTTPClient == nil {
		r.HTTPClient = &http.Client{}
	}
	if r.StatFS == nil {
		r.StatFS = statFilesystem
	}
	if r.Now == nil {
		r.Now = time.Now
	}
}

func normalizeOptions(opts Options) Options {
	defaults := DefaultOptions()
	if strings.TrimSpace(opts.InstallRoot) == "" {
		opts.InstallRoot = defaults.InstallRoot
	}
	if strings.TrimSpace(opts.ConfigRoot) == "" {
		opts.ConfigRoot = defaults.ConfigRoot
	}
	if strings.TrimSpace(opts.SystemdUnit) == "" {
		opts.SystemdUnit = defaults.SystemdUnit
	}
	if opts.HTTPTimeout <= 0 {
		opts.HTTPTimeout = defaults.HTTPTimeout
	}
	opts.InstallRoot = filepath.Clean(opts.InstallRoot)
	opts.ConfigRoot = filepath.Clean(opts.ConfigRoot)
	opts.SystemdUnit = filepath.Clean(opts.SystemdUnit)
	return opts
}

func checkDeploymentConfig(opts Options) (*deploymentConfig, Check) {
	path := filepath.Join(opts.ConfigRoot, "stagecore.env")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, Check{ID: "deployment.config", Status: Blocker, MessageKey: "config.unreadable", Detail: path + ": " + err.Error(), RemedyKey: "remedy.config"}
	}
	values, err := parseEnvironment(content)
	if err != nil {
		return nil, Check{ID: "deployment.config", Status: Blocker, MessageKey: "config.invalid", Detail: err.Error(), RemedyKey: "remedy.config"}
	}
	cfg := &deploymentConfig{
		DataRoot:   strings.TrimSpace(values["STAGECORE_DATA_ROOT"]),
		VaultRoot:  strings.TrimSpace(values["STAGECORE_VAULT_ROOT"]),
		Listen:     strings.TrimSpace(values["STAGECORE_LISTEN"]),
		PluginPath: strings.TrimSpace(values["STAGECORE_OSC_PLUGIN_PATH"]),
	}
	missing := make([]string, 0, 4)
	for key, value := range map[string]string{
		"STAGECORE_DATA_ROOT":       cfg.DataRoot,
		"STAGECORE_VAULT_ROOT":      cfg.VaultRoot,
		"STAGECORE_LISTEN":          cfg.Listen,
		"STAGECORE_OSC_PLUGIN_PATH": cfg.PluginPath,
	} {
		if value == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, Check{ID: "deployment.config", Status: Blocker, MessageKey: "config.missing", MessageArgs: []string{strings.Join(missing, ", ")}, RemedyKey: "remedy.config"}
	}
	if !filepath.IsAbs(cfg.DataRoot) || !filepath.IsAbs(cfg.VaultRoot) || !filepath.IsAbs(cfg.PluginPath) {
		return nil, Check{ID: "deployment.config", Status: Blocker, MessageKey: "config.paths", RemedyKey: "remedy.config"}
	}
	if filepath.Clean(cfg.DataRoot) == filepath.Clean(cfg.VaultRoot) {
		return nil, Check{ID: "deployment.config", Status: Blocker, MessageKey: "config.roots_same", RemedyKey: "remedy.config"}
	}
	if _, _, err := splitHostPort(cfg.Listen); err != nil {
		return nil, Check{ID: "deployment.config", Status: Blocker, MessageKey: "config.listen", Detail: err.Error(), RemedyKey: "remedy.config"}
	}
	expectedPlugin := filepath.Join(opts.InstallRoot, "bin", "stagecore-osc-plugin")
	status := Ready
	message := "config.ok"
	remedy := ""
	detail := fmt.Sprintf("data=%s vault=%s listen=%s", cfg.DataRoot, cfg.VaultRoot, cfg.Listen)
	if filepath.Clean(cfg.PluginPath) != filepath.Clean(expectedPlugin) {
		status = Warning
		message = "config.plugin_path"
		remedy = "remedy.config.plugin_path"
		detail = fmt.Sprintf("configured=%s expected=%s", cfg.PluginPath, expectedPlugin)
	}
	return cfg, Check{ID: "deployment.config", Status: status, MessageKey: message, Detail: detail, RemedyKey: remedy}
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
			return nil, fmt.Errorf("environment key is empty")
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

func checkBinaries(installRoot string) Check {
	binRoot := filepath.Join(installRoot, "bin")
	bad := make([]string, 0)
	for _, name := range deployment.RequiredBinaries {
		path := filepath.Join(binRoot, name)
		info, err := os.Lstat(path)
		if err != nil {
			bad = append(bad, name+": "+err.Error())
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			bad = append(bad, name+": not a regular executable")
		}
	}
	if len(bad) != 0 {
		return Check{ID: "deployment.binaries", Status: Blocker, MessageKey: "binaries.bad", Detail: strings.Join(bad, "; "), RemedyKey: "remedy.binaries"}
	}
	return Check{ID: "deployment.binaries", Status: Ready, MessageKey: "binaries.ok", Detail: binRoot}
}

func checkSystemdUnit(opts Options) (bool, Check) {
	content, err := os.ReadFile(opts.SystemdUnit)
	if err != nil {
		return false, Check{ID: "systemd.unit", Status: Blocker, MessageKey: "unit.unreadable", Detail: opts.SystemdUnit + ": " + err.Error(), RemedyKey: "remedy.systemd.unit"}
	}
	text := string(content)
	expectedExec := "ExecStart=" + filepath.Join(opts.InstallRoot, "bin", "stagecore-hub")
	expectedEnv := "EnvironmentFile=" + filepath.Join(opts.ConfigRoot, "stagecore.env")
	missing := make([]string, 0, 2)
	for _, marker := range []string{expectedExec, expectedEnv} {
		if !strings.Contains(text, marker) {
			missing = append(missing, marker)
		}
	}
	if len(missing) > 0 {
		return false, Check{ID: "systemd.unit", Status: Blocker, MessageKey: "unit.mismatch", Detail: strings.Join(missing, "; "), RemedyKey: "remedy.systemd.unit"}
	}
	return true, Check{ID: "systemd.unit", Status: Ready, MessageKey: "unit.ok", Detail: opts.SystemdUnit}
}

func (r *Runner) checkSystemdEnabled(ctx context.Context) Check {
	output, err := r.Command(ctx, "systemctl", "is-enabled", "stagecore-hub.service")
	state := strings.TrimSpace(string(output))
	if err == nil && state == "enabled" {
		return Check{ID: "systemd.enabled", Status: Ready, MessageKey: "service.enabled"}
	}
	if state == "disabled" {
		return Check{ID: "systemd.enabled", Status: Warning, MessageKey: "service.disabled", RemedyKey: "remedy.service.enable"}
	}
	if state == "" {
		state = errString(err)
	}
	return Check{ID: "systemd.enabled", Status: Warning, MessageKey: "service.enabled_unknown", Detail: state, RemedyKey: "remedy.service.enable"}
}

func (r *Runner) checkSystemdActive(ctx context.Context) Check {
	output, err := r.Command(ctx, "systemctl", "is-active", "stagecore-hub.service")
	state := strings.TrimSpace(string(output))
	if err == nil && state == "active" {
		return Check{ID: "systemd.active", Status: Ready, MessageKey: "service.active"}
	}
	if state == "" {
		state = errString(err)
	}
	return Check{ID: "systemd.active", Status: Blocker, MessageKey: "service.inactive", MessageArgs: []string{state}, RemedyKey: "remedy.hub.active"}
}

func (r *Runner) checkStorage(id, path string) Check {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		detail := path
		if err != nil {
			detail += ": " + err.Error()
		}
		return Check{ID: id, Status: Blocker, MessageKey: "storage.unavailable", Detail: detail, RemedyKey: "remedy.storage"}
	}
	fs, err := r.StatFS(path)
	if err != nil {
		return Check{ID: id, Status: Blocker, MessageKey: "storage.unavailable", Detail: path + ": " + err.Error(), RemedyKey: "remedy.storage"}
	}
	reserve := uint64(storagehealth.DefaultRuntimeReserveBytes)
	freePercent := float64(0)
	if fs.TotalBytes > 0 {
		freePercent = float64(fs.FreeBytes) * 100 / float64(fs.TotalBytes)
	}
	detail := fmt.Sprintf("path=%s free=%s total=%s free_percent=%.1f reserve=%s", path, humanBytes(fs.FreeBytes), humanBytes(fs.TotalBytes), freePercent, humanBytes(reserve))
	if fs.FreeBytes < reserve {
		return Check{ID: id, Status: Blocker, MessageKey: "storage.reserve", Detail: detail, RemedyKey: "remedy.storage.free"}
	}
	if freePercent < storagehealth.DefaultWarningPercent {
		return Check{ID: id, Status: Warning, MessageKey: "storage.low", Detail: detail, RemedyKey: "remedy.storage.free"}
	}
	return Check{ID: id, Status: Ready, MessageKey: "storage.ok", Detail: detail}
}

func inspectDatabase(ctx context.Context, dataRoot string) (*sql.DB, Check) {
	path := filepath.Join(dataRoot, "db", db.FileName)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		detail := path
		if err != nil {
			detail += ": " + err.Error()
		}
		return nil, Check{ID: "database.readonly", Status: Blocker, MessageKey: "database.unavailable", Detail: detail, RemedyKey: "remedy.database"}
	}

	database, err := sql.Open("sqlite", readOnlySQLiteDSN(path))
	if err != nil {
		return nil, Check{ID: "database.readonly", Status: Blocker, MessageKey: "database.unavailable", Detail: err.Error(), RemedyKey: "remedy.database"}
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	closeWith := func(check Check) (*sql.DB, Check) {
		_ = database.Close()
		return nil, check
	}
	if err := database.PingContext(ctx); err != nil {
		return closeWith(Check{ID: "database.readonly", Status: Blocker, MessageKey: "database.unavailable", Detail: err.Error(), RemedyKey: "remedy.database"})
	}
	var one int
	if err := database.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		if err == nil {
			err = errors.New("unexpected SELECT 1 result")
		}
		return closeWith(Check{ID: "database.readonly", Status: Blocker, MessageKey: "database.query", Detail: err.Error(), RemedyKey: "remedy.database"})
	}
	var quickCheck string
	if err := database.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&quickCheck); err != nil || !strings.EqualFold(strings.TrimSpace(quickCheck), "ok") {
		if err == nil {
			err = fmt.Errorf("quick_check=%q", quickCheck)
		}
		return closeWith(Check{ID: "database.readonly", Status: Blocker, MessageKey: "database.integrity", Detail: err.Error(), RemedyKey: "remedy.database"})
	}
	var version int64
	if err := database.QueryRowContext(ctx, "SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = 1").Scan(&version); err != nil {
		return closeWith(Check{ID: "database.readonly", Status: Blocker, MessageKey: "database.schema", Detail: err.Error(), RemedyKey: "remedy.database"})
	}
	return database, Check{ID: "database.readonly", Status: Ready, MessageKey: "database.ok", MessageArgs: []string{strconv.FormatInt(version, 10)}, Detail: path}
}

func inspectPairing(ctx context.Context, database *sql.DB) Check {
	rows, err := database.QueryContext(ctx, `SELECT trust_state, readiness, COUNT(*) FROM companions GROUP BY trust_state, readiness`)
	if err != nil {
		return Check{ID: "pairing.summary", Status: Warning, MessageKey: "pairing.unavailable", Detail: err.Error(), RemedyKey: "remedy.pairing"}
	}
	defer rows.Close()
	var trustedReady, trustedUnready, untrusted, revoked int
	for rows.Next() {
		var trust, readiness string
		var count int
		if err := rows.Scan(&trust, &readiness, &count); err != nil {
			return Check{ID: "pairing.summary", Status: Warning, MessageKey: "pairing.unavailable", Detail: err.Error(), RemedyKey: "remedy.pairing"}
		}
		switch trust {
		case "TRUSTED":
			if readiness == "READY" {
				trustedReady += count
			} else {
				trustedUnready += count
			}
		case "REVOKED":
			revoked += count
		default:
			untrusted += count
		}
	}
	if err := rows.Err(); err != nil {
		return Check{ID: "pairing.summary", Status: Warning, MessageKey: "pairing.unavailable", Detail: err.Error(), RemedyKey: "remedy.pairing"}
	}
	detail := fmt.Sprintf("trusted_ready=%d trusted_unready=%d untrusted=%d revoked=%d", trustedReady, trustedUnready, untrusted, revoked)
	if trustedUnready > 0 {
		return Check{ID: "pairing.summary", Status: Warning, MessageKey: "pairing.unready", MessageArgs: []string{strconv.Itoa(trustedUnready)}, Detail: detail, RemedyKey: "remedy.pairing.unready"}
	}
	return Check{ID: "pairing.summary", Status: Ready, MessageKey: "pairing.ok", Detail: detail}
}

func (r *Runner) checkHubLive(ctx context.Context, listen string, timeout time.Duration) (bool, Check) {
	readyURL, err := deployment.ReadinessURL(listen)
	if err != nil {
		return false, Check{ID: "hub.live", Status: Blocker, MessageKey: "hub.live_error", Detail: err.Error(), RemedyKey: "remedy.hub.active"}
	}
	liveURL := strings.TrimSuffix(readyURL, "/ready") + "/live"
	statusCode, payload, err := r.getHealth(ctx, liveURL, timeout)
	if err != nil {
		return false, Check{ID: "hub.live", Status: Blocker, MessageKey: "hub.live_error", Detail: err.Error(), RemedyKey: "remedy.hub.active"}
	}
	if statusCode != http.StatusOK || payload.Status != "LIVE" {
		return false, Check{ID: "hub.live", Status: Blocker, MessageKey: "hub.live_bad", Detail: fmt.Sprintf("http=%d status=%s", statusCode, payload.Status), RemedyKey: "remedy.hub.active"}
	}
	return true, Check{ID: "hub.live", Status: Ready, MessageKey: "hub.live_ok", Detail: liveURL}
}

func (r *Runner) checkHubReady(ctx context.Context, listen string, timeout time.Duration) Check {
	readyURL, err := deployment.ReadinessURL(listen)
	if err != nil {
		return Check{ID: "hub.ready", Status: Blocker, MessageKey: "hub.ready_error", Detail: err.Error(), RemedyKey: "remedy.hub.ready"}
	}
	statusCode, payload, err := r.getHealth(ctx, readyURL, timeout)
	if err != nil {
		return Check{ID: "hub.ready", Status: Blocker, MessageKey: "hub.ready_error", Detail: err.Error(), RemedyKey: "remedy.hub.ready"}
	}
	detail := fmt.Sprintf("http=%d status=%s storage_state=%s", statusCode, payload.Status, payload.StorageState)
	if statusCode == http.StatusOK && payload.Status == "READY" {
		return Check{ID: "hub.ready", Status: Ready, MessageKey: "hub.ready_ok", Detail: detail}
	}
	return Check{ID: "hub.ready", Status: Blocker, MessageKey: "hub.ready_blocked", Detail: detail, RemedyKey: "remedy.hub.ready"}
}

type healthPayload struct {
	Status       string `json:"status"`
	StorageState string `json:"storage_state"`
}

func (r *Runner) getHealth(ctx context.Context, endpoint string, timeout time.Duration) (int, healthPayload, error) {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, healthPayload{}, err
	}
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return 0, healthPayload{}, err
	}
	defer resp.Body.Close()
	var payload healthPayload
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 64<<10))
	if err := decoder.Decode(&payload); err != nil {
		return resp.StatusCode, healthPayload{}, fmt.Errorf("decode health response: %w", err)
	}
	return resp.StatusCode, payload, nil
}

func skippedCheck(id, messageKey, remedyKey string) Check {
	return Check{ID: id, Status: Advisory, MessageKey: messageKey, RemedyKey: remedyKey}
}

func finalizeReport(report *Report) {
	for _, check := range report.Checks {
		switch check.Status {
		case Ready:
			report.Counts.Ready++
		case Warning:
			report.Counts.Warning++
		case Advisory:
			report.Counts.Advisory++
		case Blocker:
			report.Counts.Blocker++
		}
	}
	switch {
	case report.Counts.Blocker > 0:
		report.Overall = OverallBlocked
	case report.Counts.Warning > 0:
		report.Overall = OverallWarning
	default:
		report.Overall = OverallReady
	}
}

func (r Report) ExitCode() int {
	if r.Overall == OverallBlocked {
		return 1
	}
	return 0
}

func readOnlySQLiteDSN(path string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := url.Values{}
	q.Set("mode", "ro")
	q.Set("_busy_timeout", "1500")
	q.Add("_pragma", "query_only(ON)")
	u.RawQuery = q.Encode()
	return u.String()
}

func execCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func statFilesystem(path string) (Filesystem, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return Filesystem{}, err
	}
	blockSize := uint64(stat.Bsize)
	return Filesystem{TotalBytes: uint64(stat.Blocks) * blockSize, FreeBytes: uint64(stat.Bavail) * blockSize}, nil
}

func splitHostPort(value string) (string, string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil || port == "" {
		return "", "", fmt.Errorf("invalid listen address %q", value)
	}
	return host, port, nil
}

func errString(err error) string {
	if err == nil {
		return "unknown"
	}
	return err.Error()
}

func humanBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}
