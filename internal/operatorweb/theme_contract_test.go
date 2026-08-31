package operatorweb

import (
	"strings"
	"testing"
)

func TestAppearanceThemeFoundationContract(t *testing.T) {
	css := string(mustReadOperatorContractFile(t, "static/theme.css"))
	js := string(mustReadOperatorContractFile(t, "static/theme.js"))

	for _, token := range []string{
		"--sc-bg-canvas",
		"--sc-bg-surface",
		"--sc-text-primary",
		"--sc-text-secondary",
		"--sc-border-default",
		"--sc-accent",
		"--sc-focus-ring",
		"--sc-success-fg",
		"--sc-success-bg",
		"--sc-warning-fg",
		"--sc-warning-bg",
		"--sc-danger-fg",
		"--sc-danger-bg",
		"--sc-selection-bg",
		"--sc-cue-focus-start",
		"--sc-cue-focus-end",
	} {
		if !strings.Contains(css, token) {
			t.Errorf("theme.css missing semantic token %q", token)
		}
	}

	for _, selector := range []string{
		`html[data-theme="dark"]`,
		`html[data-theme="light"]`,
		`html[data-accent="blue"]`,
		`html[data-accent="teal"]`,
		`html[data-accent="violet"]`,
		`html[data-accent="amber"]`,
		`@media (prefers-reduced-motion: reduce)`,
	} {
		if !strings.Contains(css, selector) {
			t.Errorf("theme.css missing %q", selector)
		}
	}

	for _, required := range []string{
		`F016_DEFAULT_MODE = "system"`,
		`F016_DEFAULT_ACCENT = "blue"`,
		`["system", "light", "dark"]`,
		`["blue", "teal", "violet", "amber"]`,
		`stagecore_appearance_mode`,
		`stagecore_appearance_accent`,
		`window.matchMedia("(prefers-color-scheme: light)")`,
		`document.documentElement.dataset.themePreference`,
		`document.documentElement.dataset.theme`,
		`document.documentElement.dataset.accent`,
		`localStorage.setItem(F016_MODE_KEY`,
		`localStorage.setItem(F016_ACCENT_KEY`,
		`localStorage.removeItem(F016_MODE_KEY)`,
		`appearance.local_scope`,
	} {
		if !strings.Contains(js, required) {
			t.Errorf("theme.js missing %q", required)
		}
	}

	if strings.Contains(js, "fetch(") || strings.Contains(js, "/api/") {
		t.Fatal("appearance foundation must not call runtime or persistence APIs")
	}
	if strings.Contains(js, "http://") || strings.Contains(js, "https://") || strings.Contains(css, "http://") || strings.Contains(css, "https://") {
		t.Fatal("appearance foundation must remain fully local/offline")
	}

	// Accent choices may change identity/focus colors, but safety semantics stay
	// on independent token families rather than aliasing to the accent.
	for _, unsafe := range []string{
		"--sc-success-fg: var(--sc-accent",
		"--sc-warning-fg: var(--sc-accent",
		"--sc-danger-fg: var(--sc-accent",
	} {
		if strings.Contains(css, unsafe) {
			t.Fatalf("safety semantic token must not alias accent: %q", unsafe)
		}
	}
}
