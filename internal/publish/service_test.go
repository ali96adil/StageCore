package publish

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestValidateRejectsInlineCredentialButAllowsSecretReference(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Credential Validation"})
	if err != nil {
		t.Fatal(err)
	}
	inline, _ := json.Marshal(map[string]any{"url": "http://device.local", "password": "should-not-publish"})
	alias, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "HTTP-DEVICE", LogicalType: "http", TargetRef: "HTTP-DEVICE", ProjectConfig: inline,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := New(s, capability.NewRegistry())
	report, err := service.Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || !hasFinding(report, "INLINE_CREDENTIAL_FORBIDDEN") {
		t.Fatalf("inline credential report=%#v", report)
	}

	refConfig, _ := json.Marshal(map[string]any{
		"url": "http://device.local", "secret_ref": "secret:device-password",
		"secret_header": "Authorization", "secret_prefix": "Bearer ",
	})
	if _, err := h.DB.ExecContext(ctx, `UPDATE project_device_aliases SET project_config_json = ? WHERE alias_id = ?`, string(refConfig), alias.ID); err != nil {
		t.Fatal(err)
	}
	report, err = service.Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid {
		t.Fatalf("secret reference should remain publishable: %#v", report)
	}
}

func hasFinding(report Report, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
