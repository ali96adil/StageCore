package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/stageaudio"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

type fakeStageAudio struct {
	calls []stageaudio.Request
	onCall func(stageaudio.Request)
}
func (a *fakeStageAudio) Call(_ context.Context, r stageaudio.Request)(stageaudio.LocalReply,error){
	a.calls=append(a.calls,r)
	if a.onCall != nil { a.onCall(r) }
	return stageaudio.LocalReply{
		Status:stageaudio.Status{MasterVolume:70,Decks:map[string]stageaudio.DeckStatus{
			"A":{Commanded:"STOPPED",Volume:70},"B":{Commanded:"STOPPED",Volume:70},
		},Qualified:false},
		Outputs:[]stageaudio.AudioOutput{{ID:"pulse/analog",Label:"Pi headphones",Kind:"ANALOG"}},
		Media:[]string{"music.mp3"},
		BluetoothDevices:[]stageaudio.BluetoothDevice{{Address:"AA:BB:CC:DD:EE:FF",Name:"speaker",Paired:true}},
	},nil
}
func TestStageAudioOfflineOptionalAndAuthenticated(t *testing.T){
	h:=newAuthHarness(t)
	s:=store.New(h.db.DB,clock.Real{})
	handler:=New(WithOperatorStageAudio(h.auth,s,nil,nil)).Handler()
	req:=httptest.NewRequest(http.MethodGet,"/api/v1/system/audio/status",nil)
	req.RemoteAddr="127.0.0.1:9123"
	out:=httptest.NewRecorder()
	handler.ServeHTTP(out,req)
	if out.Code!=http.StatusUnauthorized {t.Fatalf("anonymous status=%d",out.Code)}
	owner,err:=h.auth.Login(context.Background(),"owner",h.password,"127.0.0.1")
	if err!=nil{t.Fatal(err)}
	req=httptest.NewRequest(http.MethodGet,"/api/v1/system/audio/status",nil)
	req.RemoteAddr="127.0.0.1:9123"
	req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:owner.Token})
	out=httptest.NewRecorder();handler.ServeHTTP(out,req)
	if out.Code!=http.StatusOK || !bytes.Contains(out.Body.Bytes(),[]byte(`"available":false`)) {
		t.Fatalf("missing sidecar must degrade independently status=%d body=%s",out.Code,out.Body.String())
	}
	command:=httptest.NewRequest(http.MethodPost,"/api/v1/system/audio/command",bytes.NewReader([]byte(`{"operation":"STOP","deck":"A"}`)))
	command.RemoteAddr="127.0.0.1:9124"
	command.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:owner.Token})
	command.Header.Set(csrfHeader,owner.CSRFToken)
	resp:=httptest.NewRecorder();handler.ServeHTTP(resp,command)
	if resp.Code!=http.StatusServiceUnavailable {t.Fatalf("unconfigured audio command status=%d",resp.Code)}
}
func TestStageAudioCommandsRequireCSRFPermissionAndTypedAllowlist(t *testing.T){
	h:=newAuthHarness(t)
	s:=store.New(h.db.DB,clock.Real{})
	fake:=&fakeStageAudio{}
	handler:=New(WithOperatorStageAudio(h.auth,s,fake,nil)).Handler()
	owner,err:=h.auth.Login(context.Background(),"owner",h.password,"127.0.0.1")
	if err!=nil{t.Fatal(err)}
	request:=func(method,path,body,csrf string)*httptest.ResponseRecorder{
		t.Helper()
		req:=httptest.NewRequest(method,path,bytes.NewReader([]byte(body)))
		req.RemoteAddr="127.0.0.1:9125"
		req.Header.Set("Content-Type","application/json")
		if csrf!=""{req.Header.Set(csrfHeader,csrf)}
		req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:owner.Token})
		out:=httptest.NewRecorder()
		handler.ServeHTTP(out,req)
		return out
	}
	if out:=request(http.MethodPost,"/api/v1/system/audio/command",`{"operation":"PLAY","deck":"A"}`,"");out.Code!=http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d",out.Code)
	}
	if out:=request(http.MethodPost,"/api/v1/system/audio/command",`{"operation":"eval","deck":"A"}`,owner.CSRFToken);out.Code!=http.StatusBadRequest {
		t.Fatalf("unapproved raw operation status=%d",out.Code)
	}
	if len(fake.calls)!=0{t.Fatal("failed authorization reached audio sidecar")}
	status:=request(http.MethodGet,"/api/v1/system/audio/outputs","",owner.CSRFToken)
	if status.Code!=http.StatusOK||!bytes.Contains(status.Body.Bytes(),[]byte(`"kind":"ANALOG"`)){
		t.Fatalf("output discovery status=%d body=%s",status.Code,status.Body.String())
	}
	for _,body:=range []string{`{"operation":"PLAY","deck":"A"}`,`{"operation":"SELECT_OUTPUT","output_id":"pulse/analog"}`,
		`{"operation":"BT_CONNECT","address":"AA:BB:CC:DD:EE:FF"}`} {
		out:=request(http.MethodPost,"/api/v1/system/audio/command",body,owner.CSRFToken)
		if out.Code!=http.StatusOK{t.Fatalf("valid command=%s code=%d body=%s",body,out.Code,out.Body.String())}
	}
	if len(fake.calls)!=4{t.Fatalf("expected discovery + 3 typed calls, got %#v",fake.calls)}
	if _,err:=h.db.DB.ExecContext(context.Background(),"UPDATE local_users SET role='VIEWER' WHERE username='owner'");err!=nil{t.Fatal(err)}
	if out:=request(http.MethodPost,"/api/v1/system/audio/command",`{"operation":"STOP","deck":"A"}`,owner.CSRFToken);out.Code!=http.StatusForbidden{
		t.Fatalf("viewer could control audio status=%d",out.Code)
	}
	if len(fake.calls)!=4{t.Fatal("viewer action reached local player")}
}
func TestStageAudioCannotSendCommandsDuringActiveShow(t *testing.T){
	h:=newAuthHarness(t)
	ctx:=context.Background()
	s:=store.New(h.db.DB,clock.Real{})
	_,rev,err:=s.CreateProject(ctx,store.CreateProjectParams{Name:"StageAudio SHOW lock",CreatedBy:"owner"})
	if err!=nil{t.Fatal(err)}
	if err:=s.SetRevisionStatus(ctx,rev.ID,domain.RevisionValidated);err!=nil{t.Fatal(err)}
	published,_,err:=snapshot.NewBuilder(s).Create(ctx,rev.ID,"owner")
	if err!=nil{t.Fatal(err)}
	show,err:=s.CreateSession(ctx,published.ID,domain.SessionShow,"show blocks audio")
	if err!=nil{t.Fatal(err)}
	defer s.EndSession(ctx,show.ID,domain.SessionCompleted)
	fake:=&fakeStageAudio{}
	handler:=New(WithOperatorStageAudio(h.auth,s,fake,nil)).Handler()
	owner,err:=h.auth.Login(ctx,"owner",h.password,"127.0.0.1")
	if err!=nil{t.Fatal(err)}
	payload,_:=json.Marshal(stageaudio.Request{Operation:"PLAY",Deck:"A"})
	req:=httptest.NewRequest(http.MethodPost,"/api/v1/system/audio/command",bytes.NewReader(payload))
	req.RemoteAddr="127.0.0.1:9126";req.Header.Set(csrfHeader,owner.CSRFToken)
	req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:owner.Token})
	out:=httptest.NewRecorder();handler.ServeHTTP(out,req)
	if out.Code!=http.StatusLocked||len(fake.calls)!=0{
		t.Fatalf("SHOW must block local audio: code=%d body=%s calls=%+v",out.Code,out.Body.String(),fake.calls)
	}
}

func TestStageAudioScopedLoadUsesOnlyBoundProjectVaultVersion(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	s := store.New(h.db.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Audio Project", CreatedBy: "owner"})
	if err != nil { t.Fatal(err) }
	other, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Other Project", CreatedBy: "owner"})
	if err != nil { t.Fatal(err) }
	v, err := vault.Open(t.TempDir(), s)
	if err != nil { t.Fatal(err) }
	importAudio := func(projectID, name string) store.ManagedMedia {
		t.Helper()
		result, err := v.ImportManaged(ctx, vault.ImportParams{ProjectID: projectID, Name: name, OriginalFilename: name + ".wav"}, strings.NewReader(name))
		if err != nil { t.Fatal(err) }
		return result
	}
	ours, theirs := importAudio(project.ID, "ours"), importAudio(other.ID, "theirs")
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	published, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "owner")
	if err != nil { t.Fatal(err) }
	if _, err := s.CreateSession(ctx, published.ID, domain.SessionRehearsal, "scoped audio"); err != nil { t.Fatal(err) }
	access, err := projectaccess.New(h.db.DB)
	if err != nil { t.Fatal(err) }
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	fake := &fakeStageAudio{}
	scope := &StageAudioProjectScope{ProjectID: project.ID, Vault: v, Access: access}
	handler := New(WithScopedOperatorStageAudio(h.auth, s, fake, nil, stageaudio.NewShowFence(), scope)).Handler()
	send := func(projectID, versionID string) int {
		t.Helper()
		data, _ := json.Marshal(stageaudio.Request{Operation: "LOAD", Deck: "A", File: versionID})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/system/audio/command?project_id="+projectID, bytes.NewReader(data))
		req.RemoteAddr = "127.0.0.1:9126"
		req.Header.Set(csrfHeader, owner.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		return out.Code
	}
	if send(other.ID, ours.Version.ID) != http.StatusNotFound || send(project.ID, theirs.Version.ID) != http.StatusNotFound || len(fake.calls) != 0 {
		t.Fatal("cross-project selection reached local sidecar")
	}
	if send(project.ID, ours.Version.ID) != http.StatusOK || len(fake.calls) != 1 {
		t.Fatal("valid scoped Vault version did not reach the sidecar")
	}
	got := fake.calls[0]
	if got.Operation != "LOAD_VAULT" || got.File != ours.Version.ContentHash || got.VaultVersionID != ours.Version.ID {
		t.Fatalf("unexpected scoped sidecar request: %+v", got)
	}
}

func TestScopedStageAudioDoesNotDeliverStatusAfterGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Scoped Audio"})
	if err != nil { t.Fatal(err) }
	objects, err := vault.Open(t.TempDir(), projects)
	if err != nil { t.Fatal(err) }
	access, err := projectaccess.New(h.db.DB)
	if err != nil { t.Fatal(err) }
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	operator, err := h.auth.CreateUser(ctx, "audio-operator", h.password, userauth.RoleOperator)
	if err != nil { t.Fatal(err) }
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	set := func(permissions ...projectaccess.Permission) {
		t.Helper()
		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, permissions); err != nil {
			t.Fatal(err)
		}
	}
	set(projectaccess.ProjectView, projectaccess.MediaRead)
	sidecar := &fakeStageAudio{onCall: func(_ stageaudio.Request) { set() }}
	scope := &StageAudioProjectScope{ProjectID: project.ID, Access: access, Vault: objects}
	handler := New(WithScopedOperatorStageAudio(h.auth, projects, sidecar, nil, stageaudio.NewShowFence(), scope)).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/audio/status?project_id="+project.ID, nil)
	req.RemoteAddr = "127.0.0.1:9126"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound || len(sidecar.calls) != 1 || strings.Contains(res.Body.String(), "Pi headphones") {
		t.Fatalf("revoked browser got sidecar data: %d %s", res.Code, res.Body.String())
	}
}
