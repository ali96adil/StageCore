package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The baseline is intentionally reviewed from the CI discovery output. A
// source-level route registration change MUST fail CI until the Project ACL
// and service/device trust-boundary map is re-reviewed. This gate is an
// inventory integrity guard, NOT Project ACL enforcement or a substitute for
// the HTTP integration tests.
//
// REVIEWED SNAPSHOT: edit this digest only together with an explicit
// endpoint security review and its documentation. No production cutover.
const reviewedHTTPRouteInventorySHA256 = "aab9c66ba5953fcfca177f26f7db048ee597034ed2099b90681808bbeb3a3862"

// routeRegistration describes a registration expression, not an assumption
// about whether the handler actually enforces the desired Project scope.
type routeRegistration struct {
	Source  string
	Pattern string
}

func collectHTTPRouteRegistrations(t *testing.T) []routeRegistration {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	records := make([]routeRegistration, 0, 200)
	seen := make(map[string]struct{})
	for _, entry := range entries {
		filename := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(filename, ".go") ||
			strings.HasSuffix(filename, "_test.go") {
			continue
		}
		set := token.NewFileSet()
		parsed, err := parser.ParseFile(set, filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "Handle" &&
				selector.Sel.Name != "HandleFunc") {
				return true
			}
			pattern := ""
			if literal, ok := call.Args[0].(*ast.BasicLit); ok && literal.Kind == token.STRING {
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("unquote handler pattern %s: %v", filename, err)
				}
				pattern = value
			} else {
				// Dynamic/indirect registration patterns are *not* ignored.
				// Their AST representation is inventoried, but must be
				// separately resolved by the human route/security reviewer.
				var output bytes.Buffer
				if err := format.Node(&output, set, call.Args[0]); err != nil {
					t.Fatalf("format dynamic registration %s: %v", filename, err)
				}
				pattern = "DYNAMIC(" + output.String() + ")"
			}
			record := routeRegistration{Source: filename, Pattern: pattern}
			key := filename + "|" + pattern
			// Distinct occurrences of the same pattern in one source also
			// matter. Duplicates can indicate conditional registration.
			if _, exists := seen[key]; !exists {
				records = append(records, record)
				seen[key] = struct{}{}
			} else {
				// Record duplicate registrations with deterministic sequence
				// information instead of allowing a silent coverage bypass.
				number := 2
				for {
					dup := fmt.Sprintf("%s#%d", pattern, number)
					if _, exists := seen[filename+"|"+dup]; !exists {
						record.Pattern = dup
						records = append(records, record)
						seen[filename+"|"+dup] = struct{}{}
						break
					}
					number++
				}
			}
			return true
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Source != records[j].Source {
			return records[i].Source < records[j].Source
		}
		return records[i].Pattern < records[j].Pattern
	})
	return records
}

func TestSC024RegisteredHTTPRoutesMustRemainReviewed(t *testing.T) {
	records := collectHTTPRouteRegistrations(t)
	if len(records) < 100 {
		t.Fatalf("HTTP route audit failed closed: only %d registrations found", len(records))
	}
	lines := make([]string, 0, len(records))
	dynamicCount := 0
	for _, record := range records {
		if strings.HasPrefix(record.Pattern, "DYNAMIC(") {
			dynamicCount++
		}
		lines = append(lines, record.Source+"|"+record.Pattern)
	}
	digest := sha256.Sum256([]byte(strings.Join(lines, "\n") + "\n"))
	actual := hex.EncodeToString(digest[:])
	if actual != reviewedHTTPRouteInventorySHA256 {
		t.Logf("ROUTE_REVIEW_DIGEST=%s COUNT=%d DYNAMIC=%d", actual, len(lines), dynamicCount)
		for _, line := range lines {
			t.Logf("ROUTE_REVIEW_REGISTRATION=%s", line)
		}
		t.Fatalf("SC-024 route inventory changed or is not reviewed: update security inventory, source-route classifications, endpoint bypass tests, and the reviewed SHA together")
	}
}

// A feature-specific scoped endpoint with green tests must not be mistaken
// for a complete ACL rollout. This source gate intentionally checks the
// production assembler, rather than only a test-local httpapi.New(...).
func TestSC024NoPartialProjectACLFeatureActivationInProduction(t *testing.T) {
	guardedOptions := []string{
		"WithScopedProjectFilesRead",
		"WithScopedLocalCapsuleExports",
		"WithOwnerCapsuleImportReview",
		"WithOwnerProjectCutoverPreview",
		"WithScopedOutputWatchRead",
		"WithScopedCompanionRoleWatch",
	}
	for _, pattern := range []string{
		filepath.Join("..", "..", "cmd", "stagecore-hub", "*.go"),
		filepath.Join("..", "app", "*.go"),
	} {
		paths, err := filepath.Glob(pattern)
		if err != nil { t.Fatal(err) }
		if len(paths) == 0 { t.Fatalf("missing production source glob %s", pattern) }
		for _, filename := range paths {
			if strings.HasSuffix(filename, "_test.go") { continue }
			source, err := os.ReadFile(filename)
			if err != nil { t.Fatal(err) }
			for _, option := range guardedOptions {
				if bytes.Contains(source, []byte("httpapi."+option+"(")) ||
					bytes.Contains(source, []byte("httpapi.\n"+option+"(")) {
					t.Fatalf("partial scoped Project ACL option %q wired in production %s before complete cutover", option, filename)
				}
			}
		}
	}
}
