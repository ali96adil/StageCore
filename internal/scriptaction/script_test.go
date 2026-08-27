package scriptaction

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestScriptActionSuccessWithoutShellOrInheritedEnvironment(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := t.TempDir() + "/success.txt"
	target, _ := json.Marshal(map[string]any{
		"executable": executable,
		"arguments": []string{"-test.run=TestScriptHelperSuccess", "--", marker},
	})
	result := New().Execute(context.Background(), capability.Request{
		ExecutionID: "script-success", Capability: CapabilityKey,
		Target: &capability.Target{Ref: "SCRIPT-LOCAL", LogicalType: "script", Configuration: target},
		Parameters: json.RawMessage(`{}`), TimeoutMS: 5000,
	})
	if result.Result != domain.ExecutionCompleted || result.AckLevel != contracts.AckAccepted || result.ErrorCode != "" {
		t.Fatalf("result=%+v", result)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "1" {
		t.Fatalf("marker=%q, want one execution", data)
	}
}

func TestScriptActionTimeoutTerminatesAndDoesNotRetry(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := t.TempDir() + "/timeout.txt"
	target, _ := json.Marshal(map[string]any{
		"executable": executable,
		"arguments": []string{"-test.run=TestScriptHelperTimeout", "--", marker},
	})
	result := New().Execute(context.Background(), capability.Request{
		ExecutionID: "script-timeout", Capability: CapabilityKey,
		Target: &capability.Target{Ref: "SCRIPT-SLOW", LogicalType: "script", Configuration: target},
		Parameters: json.RawMessage(`{}`), TimeoutMS: 40,
	})
	if result.Result != domain.ExecutionTimedOut || result.ErrorCode != "SCRIPT_TIMEOUT" || result.AckLevel != contracts.AckAccepted {
		t.Fatalf("result=%+v", result)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "1" {
		t.Fatalf("marker=%q, want one execution", data)
	}
}

func TestScriptActionRequiresSecretStoreForSecretReference(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	target, _ := json.Marshal(map[string]any{"executable": executable, "secret_ref": "secret://device"})
	result := New().Execute(context.Background(), capability.Request{
		ExecutionID: "script-secret", Capability: CapabilityKey,
		Target: &capability.Target{Ref: "SCRIPT-SECRET", LogicalType: "script", Configuration: target},
	})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "SECRET_STORE_REQUIRED" || result.AckLevel != contracts.AckNone {
		t.Fatalf("result=%+v", result)
	}
}

func TestScriptHelperSuccess(t *testing.T) {
	marker, ok := helperMarker()
	if !ok {
		return
	}
	if os.Getenv("HOME") != "" || os.Getenv("PATH") != "" {
		os.Exit(31)
	}
	incrementMarker(marker)
}

func TestScriptHelperTimeout(t *testing.T) {
	marker, ok := helperMarker()
	if !ok {
		return
	}
	incrementMarker(marker)
	time.Sleep(5 * time.Second)
}

func helperMarker() (string, bool) {
	for index, value := range os.Args {
		if value == "--" && index+1 < len(os.Args) {
			return os.Args[index+1], true
		}
	}
	return "", false
}

func incrementMarker(path string) {
	value := 0
	if data, err := os.ReadFile(path); err == nil {
		value, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	_ = os.WriteFile(path, []byte(strconv.Itoa(value+1)), 0o600)
}
