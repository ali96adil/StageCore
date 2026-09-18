package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/lightingnode"
)

const profileID = "stagecore.esp32-dmx-lighting-node"

type publishedEvidence struct {
	Status                string                     `json:"status"`
	RuntimeSnapshotID     string                     `json:"runtime_snapshot_id"`
	ProjectID             string                     `json:"project_id"`
	RevisionID            string                     `json:"revision_id"`
	SnapshotVersion       int64                      `json:"snapshot_version"`
	SnapshotStatus        string                     `json:"snapshot_status"`
	SnapshotContentHash   string                     `json:"snapshot_content_hash"`
	ManifestSchemaVersion int                        `json:"manifest_schema_version"`
	DeviceID              string                     `json:"device_id"`
	ProfileID             string                     `json:"profile_id"`
	Configuration         lightingnode.Configuration `json:"configuration"`
}

type commandEvidence struct {
	Status               string          `json:"status"`
	DeviceID             string          `json:"device_id"`
	QualificationCommand string          `json:"qualification_command"`
	Result               json.RawMessage `json:"result"`
}

type persistedResult struct {
	Status  string          `json:"status"`
	Payload json.RawMessage `json:"payload"`
}

type probeEnvelope struct {
	SchemaVersion int           `json:"schema_version"`
	GeneratedAt   string        `json:"generated_at"`
	Devices       []probeDevice `json:"devices"`
}

type probeDevice struct {
	DeviceID  string        `json:"device_id"`
	ProjectID string        `json:"project_id"`
	ProfileID string        `json:"profile_id"`
	Runtime   *probeRuntime `json:"runtime"`
}

type probeRuntime struct {
	ConnectionState string          `json:"connection_state"`
	Readiness       string          `json:"readiness"`
	LastSeenAtUS    int64           `json:"last_seen_at_us"`
	Observed        json.RawMessage `json:"observed"`
}

type output struct {
	Status                    string `json:"status"`
	DeviceID                  string `json:"device_id"`
	ProjectID                 string `json:"project_id"`
	RuntimeSnapshotID         string `json:"runtime_snapshot_id"`
	SnapshotContentHash       string `json:"snapshot_content_hash"`
	ExpectedConfigurationHash string `json:"expected_configuration_hash"`
	ProbeConfigurationHash    string `json:"probe_configuration_hash"`
	StateConfigurationHash    string `json:"state_configuration_hash"`
	ConfigReadHash            string `json:"config_read_hash"`
	ConfigReadMatchesPublished bool   `json:"config_read_matches_published"`
	DMXHealthy                bool   `json:"dmx_healthy"`
	Authority                 string `json:"authority"`
	Readiness                 string `json:"readiness"`
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func readJSON(path string, out any) {
	raw, err := os.ReadFile(path)
	if err != nil {
		die("read %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		die("decode %s: %v", path, err)
	}
}

func commandPayload(path, command, deviceID string) json.RawMessage {
	var evidence commandEvidence
	readJSON(path, &evidence)
	if evidence.Status != "COMPLETED" || evidence.DeviceID != deviceID || evidence.QualificationCommand != command {
		die("%s evidence identity/status mismatch", command)
	}
	var result persistedResult
	if len(evidence.Result) == 0 || string(evidence.Result) == "null" {
		die("%s persisted result missing", command)
	}
	if err := json.Unmarshal(evidence.Result, &result); err != nil {
		die("%s persisted result invalid: %v", command, err)
	}
	if result.Status != "" && result.Status != "COMPLETED" {
		die("%s persisted result status=%s", command, result.Status)
	}
	if len(result.Payload) == 0 || string(result.Payload) == "null" {
		die("%s payload missing", command)
	}
	return result.Payload
}

func canonicalAndHash(config lightingnode.Configuration) ([]byte, string) {
	canonical, err := lightingnode.CanonicalConfiguration(config)
	if err != nil {
		die("invalid lighting configuration: %v", err)
	}
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:])
}

func main() {
	publishedPath := flag.String("published", "", "published lighting evidence JSON")
	probePath := flag.String("probe", "", "device probe JSON")
	statePath := flag.String("state-command", "", "LIGHTING_STATE_READ evidence")
	configPath := flag.String("config-command", "", "LIGHTING_CONFIG_READ evidence")
	deviceID := flag.String("device-id", "", "expected lighting device")
	projectID := flag.String("project-id", "", "expected project")
	snapshotID := flag.String("runtime-snapshot-id", "", "expected Published Runtime Snapshot")
	flag.Parse()
	for name, value := range map[string]*string{
		"published": publishedPath, "probe": probePath, "state-command": statePath,
		"config-command": configPath, "device-id": deviceID, "project-id": projectID,
		"runtime-snapshot-id": snapshotID,
	} {
		if strings.TrimSpace(*value) == "" {
			die("--%s is required", name)
		}
	}

	var published publishedEvidence
	readJSON(*publishedPath, &published)
	if published.Status != "PASS" || published.SnapshotStatus != "PUBLISHED" ||
		published.ManifestSchemaVersion < 5 || published.DeviceID != *deviceID ||
		published.ProjectID != *projectID || published.RuntimeSnapshotID != *snapshotID ||
		published.ProfileID != profileID {
		die("published lighting evidence identity/status mismatch")
	}
	if len(published.SnapshotContentHash) != 64 {
		die("published snapshot content hash missing")
	}
	expectedCanonical, expectedHash := canonicalAndHash(published.Configuration)

	var probe probeEnvelope
	readJSON(*probePath, &probe)
	if probe.SchemaVersion != 1 {
		die("unsupported probe schema")
	}
	var selected *probeDevice
	for i := range probe.Devices {
		item := &probe.Devices[i]
		if item.DeviceID == *deviceID && item.ProfileID == profileID {
			if selected != nil {
				die("lighting probe target is ambiguous")
			}
			selected = item
		}
	}
	if selected == nil || selected.ProjectID != *projectID || selected.Runtime == nil {
		die("lighting probe target/scope mismatch")
	}
	if selected.Runtime.ConnectionState != "ONLINE" || selected.Runtime.Readiness != "READY" {
		die("lighting runtime is not ONLINE/READY")
	}
	generatedAt, err := time.Parse(time.RFC3339Nano, probe.GeneratedAt)
	if err != nil {
		die("probe generated_at invalid: %v", err)
	}
	if selected.Runtime.LastSeenAtUS <= 0 {
		die("lighting last_seen_at_us missing")
	}
	lastSeen := time.UnixMicro(selected.Runtime.LastSeenAtUS).UTC()
	age := generatedAt.Sub(lastSeen)
	if age < -5*time.Second || age > 20*time.Second {
		die("lighting observation is stale: %s", age)
	}
	var probeObs lightingnode.Observation
	if err := json.Unmarshal(selected.Runtime.Observed, &probeObs); err != nil {
		die("probe observation invalid: %v", err)
	}
	if probeObs.SchemaVersion != lightingnode.SchemaVersion1 || !probeObs.DMXHealthy ||
		probeObs.BrownoutWarning || probeObs.Authority != lightingnode.AuthorityStageCore {
		die("probe observation is not healthy StageCore authority")
	}
	if probeObs.ConfigurationHash != expectedHash {
		die("probe configuration hash mismatch: got=%s want=%s", probeObs.ConfigurationHash, expectedHash)
	}

	statePayload := commandPayload(*statePath, lightingnode.CommandStateRead, *deviceID)
	var stateObs lightingnode.Observation
	if err := json.Unmarshal(statePayload, &stateObs); err != nil {
		die("LIGHTING_STATE_READ payload invalid: %v", err)
	}
	if stateObs.SchemaVersion != lightingnode.SchemaVersion1 || !stateObs.DMXHealthy ||
		stateObs.BrownoutWarning || stateObs.Authority != lightingnode.AuthorityStageCore {
		die("LIGHTING_STATE_READ did not report healthy StageCore authority")
	}
	if stateObs.ConfigurationHash != expectedHash {
		die("LIGHTING_STATE_READ configuration hash mismatch: got=%s want=%s", stateObs.ConfigurationHash, expectedHash)
	}

	configPayload := commandPayload(*configPath, lightingnode.CommandConfigRead, *deviceID)
	var installed lightingnode.Configuration
	if err := json.Unmarshal(configPayload, &installed); err != nil {
		die("LIGHTING_CONFIG_READ payload invalid: %v", err)
	}
	installedCanonical, installedHash := canonicalAndHash(installed)
	if !bytes.Equal(installedCanonical, expectedCanonical) || installedHash != expectedHash {
		die("installed lighting configuration does not match Published Runtime Snapshot")
	}

	result := output{
		Status: "PASS", DeviceID: *deviceID, ProjectID: *projectID,
		RuntimeSnapshotID: *snapshotID, SnapshotContentHash: published.SnapshotContentHash,
		ExpectedConfigurationHash: expectedHash,
		ProbeConfigurationHash: probeObs.ConfigurationHash,
		StateConfigurationHash: stateObs.ConfigurationHash,
		ConfigReadHash: installedHash, ConfigReadMatchesPublished: true,
		DMXHealthy: stateObs.DMXHealthy, Authority: string(stateObs.Authority),
		Readiness: selected.Runtime.Readiness,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		die("encode result: %v", err)
	}
	fmt.Println(string(encoded))
}
