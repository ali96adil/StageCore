package stageaudio

import (
	"fmt"
	"os"
	"strings"
)

// MediaFiles lists safe, pre-existing files in the configured local rehearsal
// folder. Only direct regular files, not symlinks or directories, are exposed.
// Web uploads / arbitrary paths are explicitly outside this prototype.
func (c *Controller) MediaFiles() ([]string, error) {
	entries, err := os.ReadDir(c.root)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot list rehearsal media", ErrBackend)
	}
	files := make([]string, 0, 32)
	for _, entry := range entries {
		if len(files) >= 256 {
			break
		}
		name := entry.Name()
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		if _, err := c.safeMediaPath(name); err == nil {
			files = append(files, name)
		}
	}
	return files, nil
}
