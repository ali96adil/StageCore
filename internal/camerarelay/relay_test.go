package camerarelay

import (
 "context"
 "encoding/json"
 "fmt"
 "io"
 "log/slog"
 "mime"
 "mime/multipart"
 "net/http"
 "net/http/httptest"
 "net/textproto"
 "strings"
 "sync/atomic"
 "testing"
 "time"
)

func testJPEG(marker byte) []byte { return []byte{0xff,0xd8,marker,0xff,0xd9} }

func fakeCamera(t *testing.T, current, maximum *atomic.Int32, failFirst bool) *httptest.Server {
 t.Helper()
 var attempts atomic.Int32
 return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,req *http.Request){
  if failFirst && attempts.Add(1)==1{http.Error(w,"not ready",503);return}
  n:=current.Add(1)
  for {
   prev:=maximum.Load()
   if n<=prev || maximum.CompareAndSwap(prev,n){break}
  }
  defer current.Add(-1)
  writer:=multipart.NewWriter(w)
  w.Header().Set("Content-Type","multipart/x-mixed-replace;boundary="+writer.Boundary())
  w.WriteHeader(200)
  ticker:=time.NewTicker(12*time.Millisecond)
  defer ticker.Stop()
  for i:=0; ; i++{
   select{
   case <-req.Context().Done():return
   case <-ticker.C:
    hdr:=make(textproto.MIMEHeader)
    hdr.Set("Content-Type","image/jpeg")
    part,err:=writer.CreatePart(hdr)
    if err!=nil{return}
    if _,err=part.Write(testJPEG(byte(i)));err!=nil{return}
    w.(http.Flusher).Flush()
   }
  }
 }))
}

func eventually(t *testing.T, cond func()bool){
 t.Helper()
 until:=time.Now().Add(4*time.Second)
 for time.Now().Before(until){
  if cond(){return}
  time.Sleep(20*time.Millisecond)
 }
 t.Fatal("condition not reached before deadline")
}

func TestFourViewersOneUpstream(t *testing.T){
 var active,peak atomic.Int32
 source:=fakeCamera(t,&active,&peak,false)
 defer source.Close()
 relay,err:=New(Config{SourceURL:source.URL,ReconnectDelay:100*time.Millisecond},slog.Default())
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background())
 done:=make(chan error,1)
 go func(){done<-relay.Run(ctx)}()
 downstream:=httptest.NewServer(relay.Handler())
 defer func(){
  cancel()
  downstream.Close()
  <-done
 }()
 client:=&http.Client{Timeout:5*time.Second}
 responses:=make([]*http.Response,0,4)
 defer func(){for _,resp:=range responses{resp.Body.Close()}}()
 for i:=0;i<4;i++{
  resp,err:=client.Get(downstream.URL+"/api/v0/stream")
  if err!=nil{t.Fatal(err)}
  if resp.StatusCode!=200{t.Fatalf("viewer %d: HTTP %d",i,resp.StatusCode)}
  responses=append(responses,resp)
  typ,params,err:=mime.ParseMediaType(resp.Header.Get("Content-Type"))
  if err!=nil||typ!="multipart/x-mixed-replace"{t.Fatalf("invalid downstream content type: %s: %v",typ,err)}
  part,err:=multipart.NewReader(resp.Body,params["boundary"]).NextPart()
  if err!=nil{t.Fatal(err)}
  image,err:=io.ReadAll(part)
  if err!=nil{t.Fatal(err)}
  if len(image)!=5||image[0]!=0xff||image[1]!=0xd8{t.Fatalf("invalid JPEG from viewer %d: %v",i,image)}
 }
 eventually(t,func()bool{
  resp,err:=client.Get(downstream.URL+"/api/v0/health")
  if err!=nil{return false}
  defer resp.Body.Close()
  // Read the real JSON keys, independent from internal struct field names.
  var data map[string]any
  if json.NewDecoder(resp.Body).Decode(&data)!=nil{return false}
  return resp.StatusCode==200 && data["viewers"]==float64(4) && data["state"]=="ready"
 })
 resp,err:=client.Get(downstream.URL+"/api/v0/stream")
 if err!=nil{t.Fatal(err)}
 resp.Body.Close()
 if resp.StatusCode!=503{t.Fatalf("fifth viewer: got %d, want 503",resp.StatusCode)}
 if peak.Load()!=1{t.Fatalf("upstream peak connections=%d, want 1",peak.Load())}
}

func TestBoundedLatestFrameAndUnsubscribe(t *testing.T){
 relay,err:=New(Config{SourceURL:"http://127.0.0.1:1/",MaxClients:1},nil)
 if err!=nil{t.Fatal(err)}
 id,ch,ok:=relay.subscribe(false)
 if !ok{t.Fatal("first subscriber rejected")}
 if _,_,ok=relay.subscribe(false);ok{t.Fatal("second subscriber exceeded limit")}
 for i:=0;i<300;i++{relay.publish(testJPEG(byte(i)))}
 select{
 case got:=<-ch:
  if got[2]!=byte(299 % 256){t.Fatalf("stale frame delivered: %v",got)}
 default:t.Fatal("subscriber received no frame")
 }
 relay.unsubscribe(id)
 if _,_,ok=relay.subscribe(false);!ok{t.Fatal("slot was not released")}
}

func TestReconnectAfterTemporaryFailure(t *testing.T){
 var active,peak atomic.Int32
 source:=fakeCamera(t,&active,&peak,true)
 defer source.Close()
 relay,err:=New(Config{SourceURL:source.URL,ReconnectDelay:100*time.Millisecond},nil)
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background())
 done:=make(chan error,1)
 go func(){done<-relay.Run(ctx)}()
 defer func(){cancel();<-done}()
 eventually(t,func()bool{
  relay.mu.Lock();defer relay.mu.Unlock()
  return relay.connected&&relay.received>0
 })
 if peak.Load()!=1{t.Fatalf("multiple upstream connections: %d",peak.Load())}
}

func TestRejectOversizeAndMalformedContent(t *testing.T){
 cases:=[]struct{name,contentType string;frame []byte}{
  {"oversize","image/jpeg",append([]byte{0xff,0xd8},append(make([]byte,2048),0xff,0xd9)...)},
  {"wrong content type","text/plain",testJPEG(1)},
  {"bad markers","image/jpeg",[]byte("not a jpeg")},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   source:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,req *http.Request){
    writer:=multipart.NewWriter(w)
    w.Header().Set("Content-Type","multipart/x-mixed-replace;boundary="+writer.Boundary())
    header:=make(textproto.MIMEHeader)
    header.Set("Content-Type",tc.contentType)
    part,_:=writer.CreatePart(header)
    _,_=part.Write(tc.frame)
    _=writer.Close()
   }))
   defer source.Close()
   relay,err:=New(Config{SourceURL:source.URL,MaxFrameBytes:1024},nil)
   if err!=nil{t.Fatal(err)}
   if err=relay.ingest(context.Background());err==nil{
    t.Fatal("malformed frame accepted")
   }
   relay.mu.Lock();count:=relay.received;relay.mu.Unlock()
   if count!=0{t.Fatalf("published %d invalid frames",count)}
  })
 }
}

func TestConfigRejectsInvalidURLAndLimits(t *testing.T){
 for _,url:=range []string{"","file:///etc/passwd","https://camera/stream","http://name:secret@camera/","http:///stream"}{
  if _,err:=New(Config{SourceURL:url},nil);err==nil{t.Fatalf("accepted %q",url)}
 }
 for _,n:=range []int{-1,17}{
  if _,err:=New(Config{SourceURL:"http://camera/stream",MaxClients:n},nil);err==nil{t.Fatal(fmt.Sprintf("accepted client limit %d",n))}
 }
 if _,err:=New(Config{SourceURL:"http://camera/stream",FlashControlURL:"http://other/api/v0/flash"},nil);err==nil{
  t.Fatal("accepted flash control on a different camera host")
 }
 if _,err:=New(Config{SourceURL:"http://camera/stream",FlashControlURL:"http://camera/wrong"},nil);err==nil{
  t.Fatal("accepted unexpected flash control path")
 }
}

func TestHealthWhenNoSource(t *testing.T){
 relay,err:=New(Config{SourceURL:"http://127.0.0.1:1/"},nil)
 if err!=nil{t.Fatal(err)}
 recorder:=httptest.NewRecorder()
 relay.Handler().ServeHTTP(recorder,httptest.NewRequest("GET","/api/v0/health",nil))
 if recorder.Code!=503 || !strings.Contains(recorder.Body.String(),"reconnecting"){t.Fatalf("unexpected health: %d %s",recorder.Code,recorder.Body.String())}
 if recorder.Header().Get("Access-Control-Allow-Origin")!="*" {
  t.Fatalf("health CORS=%q",recorder.Header().Get("Access-Control-Allow-Origin"))
 }
 var health map[string]any
 if err:=json.Unmarshal(recorder.Body.Bytes(),&health);err!=nil{t.Fatal(err)}
 if health["service"]!="stagecore-camera-relay" {
  t.Fatalf("health service=%v",health["service"])
 }
}


func TestFlashFollowsExplicitViewerRequestManualOverrideAndFailsSafeOff(t *testing.T){
 var flash atomic.Bool
 var calls atomic.Int32
 control:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,req *http.Request){
  if req.Method!=http.MethodPost || req.URL.Path!="/api/v0/flash"{http.NotFound(w,req);return}
  body,err:=io.ReadAll(io.LimitReader(req.Body,64))
  if err!=nil{t.Fatal(err)}
  switch string(body){
  case "{\"on\":true}":flash.Store(true)
  case "{\"on\":false}":flash.Store(false)
  default:http.Error(w,"bad payload",400);return
  }
  calls.Add(1)
  w.Header().Set("Content-Type","application/json")
  _,_=io.WriteString(w,fmt.Sprintf("{\"flash_on\":%t}",flash.Load()))
 }))
 defer control.Close()

 relay,err:=New(Config{
  SourceURL:"http://127.0.0.1:1/api/v0/stream",
  FlashControlURL:control.URL+"/api/v0/flash",
  ReconnectDelay:100*time.Millisecond,
  FlashTimeout:time.Second,
 },nil)
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background())
 done:=make(chan error,1)
 go func(){done<-relay.Run(ctx)}()

 // Startup AUTO with no flash-requesting viewers is fail-safe OFF.
 eventually(t,func()bool{
  relay.mu.Lock();defer relay.mu.Unlock()
  return relay.flashAppliedKnown && !relay.flashApplied && !flash.Load()
 })

 normalID,_,ok:=relay.subscribe(false)
 if !ok{t.Fatal("normal viewer rejected")}
 time.Sleep(100*time.Millisecond)
 if flash.Load(){t.Fatal("normal viewer unexpectedly enabled flash")}

 flashID,_,ok:=relay.subscribe(true)
 if !ok{t.Fatal("flash viewer rejected")}
 eventually(t,func()bool{return flash.Load()})

 // Removing the only flash requester must turn OFF even while normal Live remains.
 relay.unsubscribe(flashID)
 eventually(t,func()bool{return !flash.Load()})

 if err:=relay.setFlashMode(FlashModeOn);err!=nil{t.Fatal(err)}
 if err:=relay.reconcileFlash(context.Background());err!=nil{t.Fatal(err)}
 eventually(t,func()bool{return flash.Load()})

 flashID,_,ok=relay.subscribe(true)
 if !ok{t.Fatal("flash viewer rejected under manual ON")}

 if err:=relay.setFlashMode(FlashModeOff);err!=nil{t.Fatal(err)}
 if err:=relay.reconcileFlash(context.Background());err!=nil{t.Fatal(err)}
 eventually(t,func()bool{return !flash.Load()})

 if err:=relay.setFlashMode(FlashModeAuto);err!=nil{t.Fatal(err)}
 if err:=relay.reconcileFlash(context.Background());err!=nil{t.Fatal(err)}
 eventually(t,func()bool{return flash.Load()})

 relay.unsubscribe(flashID)
 relay.unsubscribe(normalID)
 eventually(t,func()bool{return !flash.Load()})

 recorder:=httptest.NewRecorder()
 relay.Handler().ServeHTTP(recorder,httptest.NewRequest(http.MethodGet,"/api/v0/health",nil))
 var health map[string]any
 if err:=json.Unmarshal(recorder.Body.Bytes(),&health);err!=nil{t.Fatal(err)}
 if health["flash_mode"]!="auto" || health["flash_requesting_viewers"]!=float64(0){
  t.Fatalf("unexpected flash health: %v",health)
 }

 cancel()
 if err:=<-done;err!=nil{t.Fatal(err)}
 if flash.Load(){t.Fatal("flash remained on after relay shutdown")}
 if calls.Load()<5{t.Fatalf("flash control calls=%d, expected startup/transitions/overrides",calls.Load())}
}

func TestFlashRequestQueryAndDisabledControl(t *testing.T){
 relay,err:=New(Config{SourceURL:"http://127.0.0.1:1/api/v0/stream"},nil)
 if err!=nil{t.Fatal(err)}

 rec:=httptest.NewRecorder()
 relay.Handler().ServeHTTP(rec,httptest.NewRequest(http.MethodGet,"/api/v0/stream?flash=1",nil))
 if rec.Code!=http.StatusServiceUnavailable{
  t.Fatalf("flash viewer HTTP %d, want 503",rec.Code)
 }

 rec=httptest.NewRecorder()
 relay.Handler().ServeHTTP(rec,httptest.NewRequest(http.MethodGet,"/api/v0/stream?flash=banana",nil))
 if rec.Code!=http.StatusBadRequest{
  t.Fatalf("invalid flash query HTTP %d, want 400",rec.Code)
 }

 rec=httptest.NewRecorder()
 relay.Handler().ServeHTTP(rec,httptest.NewRequest(http.MethodPost,"/api/v0/flash?state=on",nil))
 if rec.Code!=http.StatusServiceUnavailable{
  t.Fatalf("manual flash HTTP %d, want 503",rec.Code)
 }
}

func TestFlashFailureDoesNotRejectViewer(t *testing.T){
 control:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,_ *http.Request){
  http.Error(w,"flash unavailable",http.StatusServiceUnavailable)
 }))
 defer control.Close()
 relay,err:=New(Config{
  SourceURL:"http://127.0.0.1:1/api/v0/stream",
  FlashControlURL:control.URL+"/api/v0/flash",
  ReconnectDelay:100*time.Millisecond,
  FlashTimeout:time.Second,
 },nil)
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background())
 done:=make(chan error,1)
 go func(){done<-relay.Run(ctx)}()
 id,_,ok:=relay.subscribe(true)
 if !ok{t.Fatal("flash control failure rejected viewer")}
 eventually(t,func()bool{
  relay.mu.Lock();defer relay.mu.Unlock()
  return relay.flashDesired && relay.flashError!=""
 })
 relay.unsubscribe(id)
 cancel()
 <-done
}
