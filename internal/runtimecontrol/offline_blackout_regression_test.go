package runtimecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

// A disconnected Operator browser or missing Lighting node is not proof that
// outputs stopped. These regression tests use the existing Hub safety code,
// but NEVER operate DMX, OSC, MIDI, Bluetooth or tablet hardware.
func TestEmergencyBlackoutOfflineOutputKeepsDurableGoFenceAcrossHubRestart(t *testing.T) {
	h := newRuntimeHarness(t)
	ctx := context.Background()
	session, started := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000009201",
	})
	if started.Status != contracts.CommandCompleted { t.Fatalf("start: %+v", started) }

	h.service.emergencySafety = func(_ context.Context, _ domain.Session, _ contracts.CommandEnvelope, engage bool) (json.RawMessage,error) {
		if !engage { t.Fatal("unrequested emergency clear") }
		return json.RawMessage(`{"lighting":{"status":"UNCONFIRMED","physical_verified":false}}`),
			errors.New("simulated lighting controller disconnected")
	}
	result := h.service.EmergencyBlackout(ctx, EmergencyRequest{
		SessionID:session.ID,Issuer:"owner",Enabled:true,
		RequestID:"00000000-0000-7000-8000-000000009202",
	})
	if result.Status == contracts.CommandCompleted ||
		result.Error == nil || result.Error.ErrorCode != "EMERGENCY_BLACKOUT_PARTIAL_FAILURE" {
		t.Fatalf("disconnected output falsely reported successful blackout: %+v",result)
	}
	latch,err := h.store.SessionManagedOutputBlackout(ctx,session.ID)
	if err!=nil || !latch {t.Fatalf("offline safety attempt lost GO latch: %v %v",latch,err)}
	checkGoBlocked:=func(service *Service,id string) {
		t.Helper()
		got:=service.Go(context.Background(),CueRequest{SessionID:session.ID,Issuer:"owner",RequestID:id})
		if got.Status!=contracts.CommandRejected || got.Error==nil ||
			got.Error.ErrorCode!="EMERGENCY_BLACKOUT_ACTIVE" {
			t.Fatalf("GO retried while blackout result unconfirmed: %+v",got)
		}
	}
	checkGoBlocked(h.service,"00000000-0000-7000-8000-000000009203")
	// Recreate the Hub-side runtime service using the persisted DB.
	// A restart must not reset the blackout and auto-replay a previous GO.
	restarted:=New(h.store,capability.NewRegistry())
	checkGoBlocked(restarted,"00000000-0000-7000-8000-000000009204")
}

func TestOperatorCancellationDuringEmergencyDoesNotReleaseLatchedBlackout(t *testing.T) {
	h:=newRuntimeHarness(t)
	session,started:=h.service.StartSession(context.Background(),StartRequest{
		ProjectID:h.project.ID,Mode:domain.SessionRehearsal,Issuer:"owner",
		RequestID:"00000000-0000-7000-8000-000000009211",
	})
	if started.Status!=contracts.CommandCompleted {t.Fatalf("start: %+v",started)}

	ctx,cancel:=context.WithCancel(context.Background())
	defer cancel()
	h.service.emergencySafety=func(_ context.Context,_ domain.Session,_ contracts.CommandEnvelope,engage bool)(json.RawMessage,error){
		if !engage {t.Fatal("unexpected blackout clear")}
		// The browser/HTTP request has gone away *after* the Hub wrote
		// the durable latch but before the downstream output acknowledged.
		cancel()
		return json.RawMessage(`{"lighting":{"status":"UNCONFIRMED"}}`),context.Canceled
	}
	result:=h.service.EmergencyBlackout(ctx,EmergencyRequest{
		SessionID:session.ID,Issuer:"owner",Enabled:true,
		RequestID:"00000000-0000-7000-8000-000000009212",
	})
	if result.Status==contracts.CommandCompleted {
		t.Fatalf("cancelled Operator request falsely confirmed safe output: %+v",result)
	}
	latched,err:=h.store.SessionManagedOutputBlackout(context.Background(),session.ID)
	if err!=nil || !latched {t.Fatalf("cancelled request cleared durable blackout latch: %v %v",latched,err)}
	restarted:=New(h.store,capability.NewRegistry())
	goResult:=restarted.Go(context.Background(),CueRequest{
		SessionID:session.ID,Issuer:"owner",RequestID:"00000000-0000-7000-8000-000000009213",
	})
	if goResult.Status!=contracts.CommandRejected || goResult.Error==nil ||
		goResult.Error.ErrorCode!="EMERGENCY_BLACKOUT_ACTIVE" {
		t.Fatalf("lost request caused automatic GO admission after restart: %+v",goResult)
	}
}

func TestMissingBlackoutAdapterDoesNotClaimPhysicalStopSuccess(t *testing.T) {
	h:=newRuntimeHarness(t)
	session,started:=h.service.StartSession(context.Background(),StartRequest{
		ProjectID:h.project.ID,Mode:domain.SessionRehearsal,Issuer:"owner",
		RequestID:"00000000-0000-7000-8000-000000009221",
	})
	if started.Status!=contracts.CommandCompleted {t.Fatalf("start: %+v",started)}
	result:=h.service.EmergencyBlackout(context.Background(),EmergencyRequest{
		SessionID:session.ID,Issuer:"owner",Enabled:true,
		RequestID:"00000000-0000-7000-8000-000000009222",
	})
	if result.Status!=contracts.CommandFailed || result.Error==nil ||
		result.Error.ErrorCode!="EMERGENCY_BLACKOUT_UNAVAILABLE"{
		t.Fatalf("unconfigured output safety claimed success: %+v",result)
	}
	// With no adapter, no output was commanded and the Hub MUST NOT
	// claim physical safety. The separate external E-stop is required.
}
