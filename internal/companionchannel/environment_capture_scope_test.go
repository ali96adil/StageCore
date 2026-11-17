package companionchannel

import (
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/store"
)

func TestCaptureReceiptCannotBeReusedAcrossScope(t *testing.T) {
	ticket := store.CompanionUploadTicket{
		CompanionID: "mac", RuntimeSessionID: "connection-a", OperationID: "capture",
		EnvironmentManifestID: "environment-a", MachineRoleID: "role-a", RuntimeSnapshotID: "snapshot-a",
		Purpose:             store.CompanionUploadExecutionEnvironmentCapture,
		ExpectedContentHash: strings.Repeat("a", 64), ExpectedSizeBytes: 42,
		Status: store.CompanionUploadTicketCompleted,
	}
	scope := store.CreateCompanionUploadTicketParams{
		CompanionID: ticket.CompanionID, RuntimeSessionID: ticket.RuntimeSessionID, OperationID: ticket.OperationID,
		EnvironmentManifestID: ticket.EnvironmentManifestID, MachineRoleID: ticket.MachineRoleID,
		RuntimeSnapshotID: ticket.RuntimeSnapshotID, Purpose: ticket.Purpose,
		ExpectedContentHash: ticket.ExpectedContentHash, ExpectedSizeBytes: ticket.ExpectedSizeBytes,
	}
	if !environmentCaptureUploadTicketMatches(ticket, scope) {
		t.Fatal("same-scope receipt cannot be reused")
	}
	for name, mutate := range map[string]func(*store.CreateCompanionUploadTicketParams){
		"companion":   func(p *store.CreateCompanionUploadTicketParams) { p.CompanionID = "other" },
		"connection":  func(p *store.CreateCompanionUploadTicketParams) { p.RuntimeSessionID = "other" },
		"operation":   func(p *store.CreateCompanionUploadTicketParams) { p.OperationID = "other" },
		"environment": func(p *store.CreateCompanionUploadTicketParams) { p.EnvironmentManifestID = "other" },
		"role":        func(p *store.CreateCompanionUploadTicketParams) { p.MachineRoleID = "other" },
		"snapshot":    func(p *store.CreateCompanionUploadTicketParams) { p.RuntimeSnapshotID = "other" },
		"purpose":     func(p *store.CreateCompanionUploadTicketParams) { p.Purpose = "other" },
		"hash":        func(p *store.CreateCompanionUploadTicketParams) { p.ExpectedContentHash = strings.Repeat("b", 64) },
		"size":        func(p *store.CreateCompanionUploadTicketParams) { p.ExpectedSizeBytes++ },
	} {
		t.Run(name, func(t *testing.T) {
			other := scope
			mutate(&other)
			if environmentCaptureUploadTicketMatches(ticket, other) {
				t.Fatal("receipt accepted a different capture scope")
			}
		})
	}
}
