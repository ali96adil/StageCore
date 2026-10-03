package httpapi

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestLiteralHTTPRoutePatternsAreUnique(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	routePattern := regexp.MustCompile(`HandleFunc\("([^"]+)"`)
	seen := map[string]string{}

	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(file)
		line := 0
		for scanner.Scan() {
			line++
			match := routePattern.FindStringSubmatch(scanner.Text())
			if len(match) != 2 {
				continue
			}
			pattern := match[1]
			location := path + ":" + strconv.Itoa(line)
			if previous, ok := seen[pattern]; ok {
				_ = file.Close()
				t.Fatalf("duplicate HTTP route pattern %q: %s and %s", pattern, previous, location)
			}
			seen[pattern] = location
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
