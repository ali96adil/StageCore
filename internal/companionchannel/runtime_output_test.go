package companionchannel

import (
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestResultFromWirePreservesStructuredOutput(t *testing.T) {
	wire := runtimeExecutionResult{
		Type:            "execution.result",
		SchemaVersion:   runtimeSchemaVersion,
		ExecutionID:     "exec-output",
		Status:          "COMPLETED",
		AckLevel:        "ACCEPTED",
		ResponseSummary: "operation completed",
		Output:          json.RawMessage(`{"adapter_key":"test.environment","operation_kind":"OPEN"}`),
	}
	result := resultFromWire(wire)
	if result.Result != domain.ExecutionCompleted || result.AckLevel != contracts.AckAccepted {
		t.Fatalf("result=%#v", result)
	}
	var output map[string]string
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatal(err)
	}
	if output["adapter_key"] != "test.environment" || output["operation_kind"] != "OPEN" {
		t.Fatalf("output=%v", output)
	}
}

func TestResultFromWireRejectsNonObjectOutput(t *testing.T) {
	result := resultFromWire(runtimeExecutionResult{
		ExecutionID: "exec-invalid-output",
		Status:      "COMPLETED",
		AckLevel:    "ACCEPTED",
		Output:      json.RawMessage(`["not-an-object"]`),
	})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "COMPANION_RESULT_INVALID" {
		t.Fatalf("result=%#v", result)
	}
}
