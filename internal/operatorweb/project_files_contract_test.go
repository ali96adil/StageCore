package operatorweb

import (
	"strings"
	"testing"
)

func TestProjectFilesProgressiveScopedUIContract(t *testing.T) {
	index, err := Read("index.html")
	if err != nil { t.Fatal(err) }
	app, err := Read("app.js")
	if err != nil { t.Fatal(err) }
	files, err := Read("project-files.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(index), `<script src="/project-files.js" defer></script>`) {
		t.Fatal("Operator shell does not load Project Files UI")
	}
	for _, required := range []string{
		`if (page === "files") await renderScopedProjectFiles();`,
		`scFilesUpdateForProject(state.project?.project_id || "")`,
	} {
		if !strings.Contains(string(app),required) {
			t.Fatalf("workspace has not integrated Files route/visibility: %s",required)
		}
	}
	source := string(files)
	for _, required := range []string{
		`scFilesNav.className = "nav-button hidden"`,
		`files/availability`,
		`/api/v1/projects/`,
		`payload?.read_only`,
		`payload?.scoped`,
		`workspace?.project_id !== projectID`,
		`state.project?.project_id !== projectID`,
		`"ar-IQ":`,
		`en:`,
		`الملفات`,
		`read-only`,
	} {
		if !strings.Contains(source,required) {
			t.Errorf("Project Files safety/locale contract missing %q",required)
		}
	}
	for _, forbidden := range []string{
		`/api/v1/vault/objects/`,
		`window.location.href`,
		`/api/v1/projects"`,
		`fetch("/api/v1/show-capsules"`,
	} {
		if strings.Contains(source,forbidden) {
			t.Errorf("Files preview must not fall back to a global/risky endpoint: %q",forbidden)
		}
	}
	if !strings.Contains(source,`esc(item.media_asset_name)`) ||
		!strings.Contains(source,`esc(item.original_filename || "—")`) {
		t.Fatal("Project file names must be HTML-escaped")
	}
}
