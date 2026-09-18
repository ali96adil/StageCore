package qualificationdevice

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/devicechannel"
)

type fakeRuntime struct {
	calls int
}

func (f *fakeRuntime) QualificationExchange(_ context.Context, deviceID string, envelope contracts.CommandEnvelope) (devicechannel.QualificationExchangeResult, error) {
	f.calls++
	return devicechannel.QualificationExchangeResult{
		CommandID: envelope.CommandID,
		Statuses:  []contracts.CommandStatus{contracts.CommandCompleted},
		Status:    contracts.CommandCompleted,
		Payload:   json.RawMessage(`{"ok":true}`),
	}, nil
}

func TestUnixQualificationServerIsLocalBoundedAndMode0600(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "qualification.sock")
	fake := &fakeRuntime{}
	server, err := Start(ctx, fake, path)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode=%o want=600", info.Mode().Perm())
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	now := time.Now().UTC()
	deadline := now.Add(time.Second)
	body, _ := json.Marshal(requestBody{
		DeviceID: "lighting-01",
		Command: contracts.CommandEnvelope{
			CommandID: "qualification-test-1", CommandType: "LIGHTING_STATE_READ",
			SchemaVersion: contracts.SchemaVersion1, IssuedAt: now, DeadlineAt: &deadline,
			ProjectID: "project-1", Issuer: "qualification:physical-runner", Priority: "P2",
			Payload: json.RawMessage(`{}`),
		},
	})
	req, _ := http.NewRequest(http.MethodPost, "http://unix/v1/device-envelope", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || fake.calls != 1 {
		t.Fatalf("status=%d calls=%d", resp.StatusCode, fake.calls)
	}

	var configRead requestBody
	_ = json.Unmarshal(body, &configRead)
	configRead.Command.CommandID = "qualification-test-config"
	configRead.Command.CommandType = "LIGHTING_CONFIG_READ"
	configRead.Command.Payload = json.RawMessage(`{}`)
	configRaw, _ := json.Marshal(configRead)
	configReq, _ := http.NewRequest(http.MethodPost, "http://unix/v1/device-envelope", bytes.NewReader(configRaw))
	configResp, err := client.Do(configReq)
	if err != nil {
		t.Fatal(err)
	}
	defer configResp.Body.Close()
	if configResp.StatusCode != http.StatusOK || fake.calls != 2 {
		t.Fatalf("config-read status=%d calls=%d", configResp.StatusCode, fake.calls)
	}

	var invalid requestBody
	_ = json.Unmarshal(body, &invalid)
	invalid.Command.CommandType = "LIGHTING_CONFIG_APPLY"
	raw, _ := json.Marshal(invalid)
	req2, _ := http.NewRequest(http.MethodPost, "http://unix/v1/device-envelope", bytes.NewReader(raw))
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest || fake.calls != 2 {
		t.Fatalf("invalid status=%d calls=%d", resp2.StatusCode, fake.calls)
	}
}
