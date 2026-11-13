package deployment

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const SystemdUnitDirectory = "/etc/systemd/system"

var serviceUnitNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]*\.service$`)

// SystemdServiceName derives the only unit name that may be controlled for a
// deployment. Unit names are passed as individual argv elements; shell syntax
// is never interpreted.
func SystemdServiceName(unitPath string) (string, error) {
	unitPath = strings.TrimSpace(unitPath)
	if unitPath == "" || !filepath.IsAbs(unitPath) || hasParentPathComponent(unitPath) {
		return "", fmt.Errorf("systemd unit path must be an absolute path without parent traversal")
	}
	name := filepath.Base(filepath.Clean(unitPath))
	if len(name) > 255 || !serviceUnitNamePattern.MatchString(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid systemd service unit name %q", name)
	}
	return name, nil
}

func IsolatedService(opts Options) bool {
	return opts.ServiceName != "" && opts.ServiceName != DefaultServiceName
}

// ValidateIsolatedInstallPaths prevents a custom service from reading or
// writing any production installation root. The candidate bundle may live
// under the candidate install root, but not inside its managed bin directory.
func ValidateIsolatedInstallPaths(opts Options) error {
	if !IsolatedService(opts) {
		return nil
	}
	name, err := SystemdServiceName(opts.SystemdUnit)
	if err != nil {
		return err
	}
	if name != opts.ServiceName {
		return fmt.Errorf("isolated service name %q must match unit filename %q", opts.ServiceName, name)
	}
	if opts.ServiceName == DefaultServiceName {
		return fmt.Errorf("isolated deployment cannot target %s", DefaultServiceName)
	}
	paths := map[string]string{
		"bundle":        opts.BundleDir,
		"install":       opts.InstallRoot,
		"configuration": opts.ConfigRoot,
		"data":          opts.DataRoot,
		"vault":         opts.VaultRoot,
	}
	for label, path := range paths {
		if !filepath.IsAbs(path) || hasParentPathComponent(path) {
			return fmt.Errorf("isolated %s path must be absolute and contain no parent traversal", label)
		}
		if err := rejectSymlinkPath(path); err != nil {
			return fmt.Errorf("isolated %s path: %w", label, err)
		}
	}
	if opts.SystemdUnit == DefaultUnitPath {
		return fmt.Errorf("isolated deployment cannot target the production systemd unit")
	}
	if err := rejectSymlinkPath(opts.SystemdUnit); err != nil {
		return fmt.Errorf("isolated systemd unit path: %w", err)
	}
	if pathOverlaps(opts.DataRoot, opts.VaultRoot) {
		return fmt.Errorf("isolated Data Root and Vault Root must not overlap")
	}
	managed := []struct {
		name string
		path string
	}{
		{"install", opts.InstallRoot},
		{"configuration", opts.ConfigRoot},
		{"data", opts.DataRoot},
		{"vault", opts.VaultRoot},
	}
	for i := range managed {
		for j := i + 1; j < len(managed); j++ {
			if pathOverlaps(managed[i].path, managed[j].path) {
				return fmt.Errorf("isolated %s and %s paths must not overlap", managed[i].name, managed[j].name)
			}
		}
	}
	for _, productionRoot := range []string{
		DefaultInstallRoot,
		DefaultConfigRoot,
		"/var/lib/stagecore",
		"/var/backups/stagecore",
	} {
		for label, path := range paths {
			if pathOverlaps(path, productionRoot) {
				return fmt.Errorf("isolated %s path overlaps protected production root %s", label, productionRoot)
			}
		}
	}
	if pathOverlaps(opts.SystemdUnit, DefaultUnitPath) {
		return fmt.Errorf("isolated systemd unit path overlaps the production unit")
	}
	if pathOverlaps(opts.BundleDir, filepath.Join(opts.InstallRoot, "bin")) {
		return fmt.Errorf("isolated release bundle must be outside the managed bin directory")
	}
	return nil
}

func ValidateIsolatedUpdatePaths(opts Options, backupRoot string) error {
	if !IsolatedService(opts) {
		return nil
	}
	if err := ValidateIsolatedInstallPaths(opts); err != nil {
		return err
	}
	if !filepath.IsAbs(backupRoot) || hasParentPathComponent(backupRoot) {
		return fmt.Errorf("isolated rollback root must be absolute and contain no parent traversal")
	}
	if err := rejectSymlinkPath(backupRoot); err != nil {
		return fmt.Errorf("isolated rollback root: %w", err)
	}
	for _, protectedRoot := range []string{"/var/backups/stagecore", "/var/lib/stagecore"} {
		if pathOverlaps(backupRoot, protectedRoot) {
			return fmt.Errorf("isolated rollback root overlaps protected production root %s", protectedRoot)
		}
	}
	for _, target := range []string{opts.BundleDir, opts.InstallRoot, opts.ConfigRoot, opts.DataRoot, opts.VaultRoot, opts.SystemdUnit} {
		if pathOverlaps(backupRoot, target) {
			return fmt.Errorf("isolated rollback root overlaps another deployment path")
		}
	}
	return nil
}

// ValidateSystemdOperationPath constrains real systemd mutations to the
// canonical system unit directory and the filename-derived target.
func ValidateSystemdOperationPath(opts Options) error {
	name, err := SystemdServiceName(opts.SystemdUnit)
	if err != nil {
		return err
	}
	if name != opts.ServiceName {
		return fmt.Errorf("systemd target name does not match unit path")
	}
	want := filepath.Join(SystemdUnitDirectory, name)
	if filepath.Clean(opts.SystemdUnit) != want {
		return fmt.Errorf("systemd operations require unit path %s", want)
	}
	if err := rejectSymlinkPath(opts.SystemdUnit); err != nil {
		return err
	}
	if info, statErr := os.Lstat(opts.SystemdUnit); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("systemd unit %s must be a regular non-symlink file", opts.SystemdUnit)
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect systemd unit %s: %w", opts.SystemdUnit, statErr)
	}
	return nil
}

func pathOverlaps(a, b string) bool {
	canonicalA, errA := canonicalPath(a)
	canonicalB, errB := canonicalPath(b)
	if errA != nil || errB != nil {
		return true
	}
	return pathWithin(canonicalA, canonicalB) || pathWithin(canonicalB, canonicalA)
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func hasParentPathComponent(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func rejectSymlinkPath(path string) error {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) || hasParentPathComponent(path) {
		return fmt.Errorf("path must be absolute and contain no parent traversal")
	}
	if info, err := os.Lstat(cleaned); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path %s is a symlink", cleaned)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect path %s: %w", cleaned, err)
	}
	_, err := canonicalPath(cleaned)
	return err
}

func canonicalPath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) || hasParentPathComponent(path) {
		return "", fmt.Errorf("path must be absolute and contain no parent traversal")
	}
	root := filepath.VolumeName(cleaned) + string(filepath.Separator)
	current := root
	pending := splitPathComponents(strings.TrimPrefix(cleaned, root))
	symlinkHops := 0
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		candidate := filepath.Join(current, part)
		info, err := os.Lstat(candidate)
		if err != nil {
			if os.IsPermission(err) {
				current = candidate
				for _, rest := range pending {
					current = filepath.Join(current, rest)
				}
				return filepath.Clean(current), nil
			}
			if !os.IsNotExist(err) {
				return "", fmt.Errorf("inspect path component %s: %w", candidate, err)
			}
			current = candidate
			for _, rest := range pending {
				current = filepath.Join(current, rest)
			}
			return filepath.Clean(current), nil
		}
		if info.Mode()&os.ModeSymlink == 0 {
			current = candidate
			continue
		}
		symlinkHops++
		if symlinkHops > 40 {
			return "", fmt.Errorf("too many symlinks while resolving %s", path)
		}
		target, err := os.Readlink(candidate)
		if err != nil {
			return "", fmt.Errorf("read symlink %s: %w", candidate, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(current, target)
		}
		target = filepath.Clean(target)
		root = filepath.VolumeName(target) + string(filepath.Separator)
		current = root
		pending = append(splitPathComponents(strings.TrimPrefix(target, root)), pending...)
	}
	return filepath.Clean(current), nil
}

func splitPathComponents(path string) []string {
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" && part != "." {
			result = append(result, part)
		}
	}
	return result
}
