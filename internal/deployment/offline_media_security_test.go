package deployment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineMediaRejectsChecksumPathTraversal(t *testing.T) {
	media := createTestOfflineMedia(t)
	malicious := strings.Repeat("0", 64) + "  ../outside\n"
	if err := os.WriteFile(filepath.Join(media, "MEDIA_SHA256SUMS"), []byte(malicious), 0o644); err != nil {
		t.Fatalf("write malicious media manifest: %v", err)
	}

	output, err := runOfflineMedia(t, media, nil, "verify")
	if err == nil {
		t.Fatalf("path-traversal manifest unexpectedly verified: %s", output)
	}
	if !strings.Contains(output, "unsafe or malformed checksum manifest") {
		t.Fatalf("unexpected traversal rejection: %s", output)
	}
}
