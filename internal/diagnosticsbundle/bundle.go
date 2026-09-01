package diagnosticsbundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/deployment"
	"github.com/ali96adil/StageCore/internal/doctor"
)

const (
	ManifestSchemaVersion = 1
	DefaultJournalLines    = 2000
	MaxJournalLines        = 20000
	MaxJournalBytes        = 4 << 20
)

type Options struct {
	OutputPath  string
	InstallRoot string
	ConfigRoot  string
	SystemdUnit string
	HTTPTimeout time.Duration
	JournalLines int
}

type Result struct {
	Path     string
	Manifest Manifest
}

type Manifest struct {
	SchemaVersion    int               `json:"schema_version"`
	GeneratedAt      time.Time         `json:"generated_at"`
	ArchiveFormat    string            `json:"archive_format"`
	DoctorOverall    doctor.Overall    `json:"doctor_overall"`
	Entries          []ManifestEntry   `json:"entries"`
	CollectionErrors []CollectionError `json:"collection_errors,omitempty"`
	Redactions       int               `json:"redactions"`
	PrivacyContract  []string          `json:"privacy_contract"`
}

type ManifestEntry struct {
	Path        string `json:"path"`
	Description string `json:"description"`
	SizeBytes   int    `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	Redacted    bool   `json:"redacted"`
}

type CollectionError struct {
	Component string `json:"component"`
	Error     string `json:"error"`
}

type bundleEntry struct {
	name        string
	description string
	data        []byte
	redacted    bool
}

type healthRunner interface {
	Run(context.Context, doctor.Options) doctor.Report
}

type commandRunner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}

type execCommands struct{}

func (execCommands) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return output, nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		return output, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, message)
}

type Service struct {
	doctor   healthRunner
	commands commandRunner
	now      func() time.Time
	hostname func() (string, error)
}

func NewService() *Service {
	return &Service{
		doctor:   doctor.NewRunner(),
		commands: execCommands{},
		now:      time.Now,
		hostname: os.Hostname,
	}
}

func DefaultOptions() Options {
	doctorDefaults := doctor.DefaultOptions()
	return Options{
		InstallRoot:  doctorDefaults.InstallRoot,
		ConfigRoot:   doctorDefaults.ConfigRoot,
		SystemdUnit:  doctorDefaults.SystemdUnit,
		HTTPTimeout:  doctorDefaults.HTTPTimeout,
		JournalLines: DefaultJournalLines,
	}
}

func (s *Service) Create(ctx context.Context, opts Options) (Result, error) {
	if s == nil {
		return Result{}, fmt.Errorf("diagnostics bundle service is required")
	}
	s.normalize()
	effective, err := normalizeOptions(opts)
	if err != nil {
		return Result{}, err
	}
	generatedAt := s.now().UTC()
	outputPath, err := resolveOutputPath(effective.OutputPath, generatedAt)
	if err != nil {
		return Result{}, err
	}

	report := s.doctor.Run(ctx, doctor.Options{
		InstallRoot: effective.InstallRoot,
		ConfigRoot:  effective.ConfigRoot,
		SystemdUnit: effective.SystemdUnit,
		HTTPTimeout: effective.HTTPTimeout,
	})
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		GeneratedAt:   generatedAt,
		ArchiveFormat: "tar+gzip",
		DoctorOverall: report.Overall,
		PrivacyContract: []string{
			"no raw StageCore database is included",
			"no raw stagecore.env is included; deployment metadata is allowlisted",
			"no Vault/media payloads are included",
			"no browser sessions, credentials, private keys, setup codes or auth records are included",
			"journal text and structured string fields pass through the diagnostics redactor",
		},
	}
	entries := make([]bundleEntry, 0, 6)

	if data, count, encodeErr := sanitizedJSON(report); encodeErr != nil {
		return Result{}, encodeErr
	} else {
		manifest.Redactions += count
		entries = append(entries, bundleEntry{name: "doctor.json", description: "Read-only StageCore Doctor report", data: data, redacted: count > 0})
	}

	systemInfo, systemErr := s.collectSystem(ctx)
	if systemErr != nil {
		s.addCollectionError(&manifest, "system", systemErr)
	}
	if data, count, encodeErr := sanitizedJSON(systemInfo); encodeErr != nil {
		return Result{}, encodeErr
	} else {
		manifest.Redactions += count
		entries = append(entries, bundleEntry{name: "system.json", description: "Host platform and StageCore build metadata", data: data, redacted: count > 0})
	}

	deploymentMetadata, dataRoot, deploymentErr := collectDeploymentMetadata(effective)
	if deploymentErr != nil {
		s.addCollectionError(&manifest, "deployment", deploymentErr)
	}
	if data, count, encodeErr := sanitizedJSON(deploymentMetadata); encodeErr != nil {
		return Result{}, encodeErr
	} else {
		manifest.Redactions += count
		entries = append(entries, bundleEntry{name: "deployment.json", description: "Allowlisted deployment metadata; raw environment file excluded", data: data, redacted: count > 0})
	}

	binaryInventory := collectBinaryInventory(effective.InstallRoot)
	if data, count, encodeErr := sanitizedJSON(binaryInventory); encodeErr != nil {
		return Result{}, encodeErr
	} else {
		manifest.Redactions += count
		entries = append(entries, bundleEntry{name: "binaries.json", description: "Managed binary presence, size, mode, architecture and SHA-256", data: data, redacted: count > 0})
	}

	if dataRoot != "" {
		stateSummary, stateErr := collectStateSummary(ctx, dataRoot)
		if stateErr != nil {
			s.addCollectionError(&manifest, "state-summary", stateErr)
		} else if data, count, encodeErr := sanitizedJSON(stateSummary); encodeErr != nil {
			return Result{}, encodeErr
		} else {
			manifest.Redactions += count
			entries = append(entries, bundleEntry{name: "state-summary.json", description: "Aggregate project/session/companion/extension state without project names, user names or auth payloads", data: data, redacted: count > 0})
		}
	}

	journal, journalErr := s.commands.Output(ctx, "journalctl",
		"--unit", "stagecore-hub.service",
		"--no-pager",
		"--output=cat",
		"--lines", fmt.Sprintf("%d", effective.JournalLines),
	)
	if journalErr != nil {
		s.addCollectionError(&manifest, "journal", journalErr)
	} else {
		journal = tailBytes(journal, MaxJournalBytes)
		journal, count := redactBytes(journal)
		manifest.Redactions += count
		entries = append(entries, bundleEntry{name: "logs/stagecore-hub.log", description: "Bounded recent stagecore-hub journal output after secret redaction", data: journal, redacted: true})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	manifest.Entries = make([]ManifestEntry, 0, len(entries))
	for _, entry := range entries {
		hash := sha256.Sum256(entry.data)
		manifest.Entries = append(manifest.Entries, ManifestEntry{
			Path: entry.name, Description: entry.description, SizeBytes: len(entry.data),
			SHA256: hex.EncodeToString(hash[:]), Redacted: entry.redacted,
		})
	}

	if err := writeArchive(outputPath, generatedAt, manifest, entries); err != nil {
		return Result{}, err
	}
	return Result{Path: outputPath, Manifest: manifest}, nil
}

func (s *Service) normalize() {
	if s.doctor == nil {
		s.doctor = doctor.NewRunner()
	}
	if s.commands == nil {
		s.commands = execCommands{}
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.hostname == nil {
		s.hostname = os.Hostname
	}
}

func normalizeOptions(opts Options) (Options, error) {
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
	if opts.JournalLines == 0 {
		opts.JournalLines = DefaultJournalLines
	}
	if opts.JournalLines < 1 || opts.JournalLines > MaxJournalLines {
		return Options{}, fmt.Errorf("journal lines must be between 1 and %d", MaxJournalLines)
	}
	for label, value := range map[string]string{
		"install root": opts.InstallRoot,
		"config root": opts.ConfigRoot,
		"systemd unit": opts.SystemdUnit,
	} {
		cleaned := filepath.Clean(strings.TrimSpace(value))
		if !filepath.IsAbs(cleaned) {
			return Options{}, fmt.Errorf("%s must be an absolute path", label)
		}
		switch label {
		case "install root":
			opts.InstallRoot = cleaned
		case "config root":
			opts.ConfigRoot = cleaned
		case "systemd unit":
			opts.SystemdUnit = cleaned
		}
	}
	return opts, nil
}

func resolveOutputPath(requested string, generatedAt time.Time) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = fmt.Sprintf("stagecore-support-%s.tar.gz", generatedAt.Format("20060102T150405Z"))
	}
	absolute, err := filepath.Abs(filepath.Clean(requested))
	if err != nil {
		return "", fmt.Errorf("resolve support bundle output path: %w", err)
	}
	if filepath.Ext(strings.TrimSuffix(absolute, ".gz")) != ".tar" || !strings.HasSuffix(strings.ToLower(absolute), ".tar.gz") {
		return "", fmt.Errorf("support bundle output must end in .tar.gz")
	}
	if _, err := os.Lstat(absolute); err == nil {
		return "", fmt.Errorf("support bundle output already exists: %s", absolute)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect support bundle output: %w", err)
	}
	return absolute, nil
}

func writeArchive(path string, generatedAt time.Time, manifest Manifest, entries []bundleEntry) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create support bundle output directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create support bundle: %w", err)
	}
	removeOnFailure := true
	defer func() {
		_ = file.Close()
		if removeOnFailure {
			_ = os.Remove(path)
		}
	}()

	gzipWriter := gzip.NewWriter(file)
	gzipWriter.Header.ModTime = generatedAt
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode support bundle manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	allEntries := append([]bundleEntry{{name: "manifest.json", description: "Support bundle manifest", data: manifestBytes}}, entries...)
	for _, entry := range allEntries {
		header := &tar.Header{
			Name: entry.name, Mode: 0o600, Size: int64(len(entry.data)),
			ModTime: generatedAt, AccessTime: generatedAt, ChangeTime: generatedAt,
			Uid: 0, Gid: 0,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("write support bundle header %s: %w", entry.name, err)
		}
		if _, err := tarWriter.Write(entry.data); err != nil {
			return fmt.Errorf("write support bundle entry %s: %w", entry.name, err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("close support bundle tar: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("close support bundle gzip: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync support bundle: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close support bundle: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("protect support bundle: %w", err)
	}
	removeOnFailure = false
	return nil
}

type SystemInfo struct {
	Hostname    string `json:"hostname,omitempty"`
	GOOS        string `json:"goos"`
	GOARCH      string `json:"goarch"`
	GoVersion   string `json:"go_version"`
	ModulePath  string `json:"module_path,omitempty"`
	VCSRevision string `json:"vcs_revision,omitempty"`
	VCSTime     string `json:"vcs_time,omitempty"`
	VCSModified string `json:"vcs_modified,omitempty"`
	Kernel      string `json:"kernel,omitempty"`
}

func (s *Service) collectSystem(ctx context.Context) (SystemInfo, error) {
	info := SystemInfo{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GoVersion: runtime.Version()}
	var errs []error
	if hostname, err := s.hostname(); err == nil {
		info.Hostname = strings.TrimSpace(hostname)
	} else {
		errs = append(errs, fmt.Errorf("hostname: %w", err))
	}
	if build, ok := debug.ReadBuildInfo(); ok && build != nil {
		info.ModulePath = build.Main.Path
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				info.VCSRevision = setting.Value
			case "vcs.time":
				info.VCSTime = setting.Value
			case "vcs.modified":
				info.VCSModified = setting.Value
			}
		}
	}
	if output, err := s.commands.Output(ctx, "uname", "-srmo"); err == nil {
		info.Kernel = strings.TrimSpace(string(output))
	} else {
		errs = append(errs, fmt.Errorf("uname: %w", err))
	}
	return info, errors.Join(errs...)
}

func collectBinaryInventory(installRoot string) []BinaryInfo {
	names := append([]string(nil), deployment.RequiredBinaries...)
	sort.Strings(names)
	items := make([]BinaryInfo, 0, len(names))
	for _, name := range names {
		path := filepath.Join(installRoot, "bin", name)
		item := BinaryInfo{Name: name, Path: path}
		info, err := os.Lstat(path)
		if err != nil {
			item.Error = err.Error()
			items = append(items, item)
			continue
		}
		item.Mode = info.Mode().String()
		item.SizeBytes = info.Size()
		if info.Mode()&os.ModeSymlink != 0 {
			item.Type = "symlink"
			items = append(items, item)
			continue
		}
		if !info.Mode().IsRegular() {
			item.Type = "other"
			items = append(items, item)
			continue
		}
		item.Type = "regular"
		if hash, err := hashFile(path); err == nil {
			item.SHA256 = hash
		} else {
			item.Error = err.Error()
		}
		if elfFile, err := elf.Open(path); err == nil {
			item.ELFMachine = elfFile.Machine.String()
			_ = elfFile.Close()
		}
		if build, err := buildinfo.ReadFile(path); err == nil && build != nil {
			for _, setting := range build.Settings {
				if setting.Key == "vcs.revision" {
					item.VCSRevision = setting.Value
					break
				}
			}
		}
		items = append(items, item)
	}
	return items
}

type BinaryInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type,omitempty"`
	Mode        string `json:"mode,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	ELFMachine  string `json:"elf_machine,omitempty"`
	VCSRevision string `json:"vcs_revision,omitempty"`
	Error       string `json:"error,omitempty"`
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Service) addCollectionError(manifest *Manifest, component string, err error) {
	if err == nil {
		return
	}
	redacted, count := RedactString(err.Error())
	manifest.Redactions += count
	manifest.CollectionErrors = append(manifest.CollectionErrors, CollectionError{Component: component, Error: redacted})
}

func tailBytes(value []byte, maximum int) []byte {
	if maximum <= 0 || len(value) <= maximum {
		return value
	}
	prefix := []byte(fmt.Sprintf("[StageCore diagnostics: journal truncated to the most recent %d bytes]\n", maximum))
	result := make([]byte, 0, len(prefix)+maximum)
	result = append(result, prefix...)
	result = append(result, value[len(value)-maximum:]...)
	return result
}
