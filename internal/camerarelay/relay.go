// Package camerarelay provides a bounded, single-upstream JPEG fan-out.
// It runs independently from the StageCore Hub and is not deployed by default.
package camerarelay

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "log/slog"
 "mime"
 "mime/multipart"
 "net"
 "net/http"
 "net/http/httptrace"
 "net/url"
 "strings"
 "sync"
 "sync/atomic"
 "time"
)

const boundary = "stagecore-relay-frame"

const (
 FlashModeAuto = "auto"
 FlashModeOn   = "on"
 FlashModeOff  = "off"
)

type subscriber struct {
 frames chan []byte
 flashRequested bool
}

type Config struct {
 SourceURL string
 FlashControlURL string
 MaxClients int
 MaxFrameBytes int64
 ReconnectDelay time.Duration
 HeaderTimeout time.Duration
 FrameTimeout time.Duration
 WriteTimeout time.Duration
 FlashTimeout time.Duration
}

type Relay struct {
 cfg Config
 log *slog.Logger
 client *http.Client
 flashClient *http.Client
 started atomic.Bool
 mu sync.Mutex
 subscribers map[uint64]subscriber
 nextID uint64
 closed bool
 connected bool
 received uint64
 lastFrame time.Time
 flashWake chan struct{}
 flashMu sync.Mutex
 flashMode string
 flashDesired bool
 flashApplied bool
 flashAppliedKnown bool
 flashError string
}

func New(cfg Config, log *slog.Logger) (*Relay, error) {
 u, err := url.Parse(cfg.SourceURL)
 if err != nil || u == nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
  return nil, errors.New("source must be HTTP with a host and no credentials or fragment")
 }
 if cfg.FlashControlURL != "" {
  flashURL, flashErr := url.Parse(cfg.FlashControlURL)
  if flashErr != nil || flashURL == nil || flashURL.Scheme != "http" || flashURL.Hostname() == "" ||
     flashURL.User != nil || flashURL.Fragment != "" || flashURL.RawQuery != "" || flashURL.Path != "/api/v0/flash" {
   return nil, errors.New("flash control must be HTTP /api/v0/flash with a host and no credentials, query or fragment")
  }
  if !strings.EqualFold(flashURL.Hostname(), u.Hostname()) {
   return nil, errors.New("flash control host must match camera source host")
  }
 }
 if cfg.MaxClients == 0 { cfg.MaxClients = 4 }
 if cfg.MaxFrameBytes == 0 { cfg.MaxFrameBytes = 512 << 10 }
 if cfg.ReconnectDelay == 0 { cfg.ReconnectDelay = time.Second }
 if cfg.HeaderTimeout == 0 { cfg.HeaderTimeout = 5*time.Second }
 if cfg.FrameTimeout == 0 { cfg.FrameTimeout = 8*time.Second }
 if cfg.WriteTimeout == 0 { cfg.WriteTimeout = 5*time.Second }
 if cfg.FlashTimeout == 0 { cfg.FlashTimeout = 2*time.Second }
 if cfg.MaxClients < 1 || cfg.MaxClients > 16 || cfg.MaxFrameBytes < 1024 ||
    cfg.MaxFrameBytes > 4<<20 || cfg.ReconnectDelay < 100*time.Millisecond ||
    cfg.HeaderTimeout < time.Second || cfg.FrameTimeout < time.Second || cfg.WriteTimeout < time.Second ||
    cfg.FlashTimeout < 500*time.Millisecond || cfg.FlashTimeout > 10*time.Second {
  return nil, errors.New("relay resource limit is out of bounds")
 }
 if log == nil { log = slog.Default() }
 transport := &http.Transport{
  Proxy: nil, // Never route a show-LAN camera via an HTTP proxy.
  DialContext: sourceDialContext,
  DisableCompression:true, DisableKeepAlives:true, MaxConnsPerHost:1,
  ResponseHeaderTimeout:cfg.HeaderTimeout,
 }
 client := &http.Client{Transport:transport, CheckRedirect:func(*http.Request,[]*http.Request)error{
  return errors.New("camera redirects are not allowed")
 }}
 flashTransport := &http.Transport{
  Proxy:nil, DialContext:sourceDialContext, DisableCompression:true,
  DisableKeepAlives:true, MaxConnsPerHost:1, ResponseHeaderTimeout:cfg.FlashTimeout,
 }
 flashClient := &http.Client{
  Transport:flashTransport, Timeout:cfg.FlashTimeout,
  CheckRedirect:func(*http.Request,[]*http.Request)error{return errors.New("camera redirects are not allowed")},
 }
 return &Relay{
  cfg:cfg, log:log, client:client, flashClient:flashClient,
  subscribers:make(map[uint64]subscriber), flashWake:make(chan struct{},1),
  flashMode:FlashModeAuto,
 }, nil
}

// Run is the sole upstream owner. Subscriber connections never contact the camera.
func (r *Relay) Run(ctx context.Context) error {
 if !r.started.CompareAndSwap(false,true) { return errors.New("relay already started") }
 if r.cfg.FlashControlURL != "" {
  go r.flashLoop(ctx)
  r.signalFlash()
 }
 defer func(){
  r.mu.Lock()
  r.connected=false
  r.closed=true
  r.flashMode=FlashModeOff
  r.flashDesired=false
  r.flashAppliedKnown=false
  for id,sub := range r.subscribers { delete(r.subscribers,id); close(sub.frames) }
  r.mu.Unlock()
  if r.cfg.FlashControlURL != "" {
   offCtx,cancel:=context.WithTimeout(context.Background(),r.cfg.FlashTimeout)
   if err:=r.reconcileFlash(offCtx);err!=nil{
    r.log.Warn("camera flash shutdown OFF failed","error",err)
   }
   cancel()
  }
  r.client.CloseIdleConnections()
  r.flashClient.CloseIdleConnections()
 }()
 for ctx.Err()==nil {
  err:=r.ingest(ctx)
  r.mu.Lock();r.connected=false;r.mu.Unlock()
  if ctx.Err()!=nil { break }
  r.log.Warn("camera disconnected; retrying", "error",err)
  select {
  case <-ctx.Done(): return nil
  case <-time.After(r.cfg.ReconnectDelay):
  }
 }
 return nil
}

func (r *Relay) ingest(ctx context.Context) error {
 var conn net.Conn
 trace:=&httptrace.ClientTrace{GotConn:func(info httptrace.GotConnInfo){conn=info.Conn}}
 req,err:=http.NewRequestWithContext(httptrace.WithClientTrace(ctx,trace),http.MethodGet,r.cfg.SourceURL,nil)
 if err!=nil{return err}
 resp,err:=r.client.Do(req)
 if err!=nil{return err}
 defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{return fmt.Errorf("camera HTTP %d",resp.StatusCode)}
 typ,params,err:=mime.ParseMediaType(resp.Header.Get("Content-Type"))
 if err!=nil || !strings.EqualFold(typ,"multipart/x-mixed-replace") || params["boundary"]=="" {
  return errors.New("upstream is not multipart MJPEG")
 }
 reader:=multipart.NewReader(resp.Body,params["boundary"])
 r.mu.Lock();r.connected=true;r.mu.Unlock()
 for ctx.Err()==nil {
  if conn!=nil { _=conn.SetReadDeadline(time.Now().Add(r.cfg.FrameTimeout)) }
  part,err:=reader.NextPart()
  if err!=nil{return err}
  typ,_,parseErr:=mime.ParseMediaType(part.Header.Get("Content-Type"))
  if parseErr!=nil || !strings.EqualFold(typ,"image/jpeg"){return errors.New("upstream part is not JPEG")}
  frame,err:=io.ReadAll(io.LimitReader(part,r.cfg.MaxFrameBytes+1))
  if err!=nil{return err}
  if int64(len(frame))>r.cfg.MaxFrameBytes{return errors.New("upstream frame too large")}
  if len(frame)<4 || frame[0]!=0xff || frame[1]!=0xd8 || frame[len(frame)-2]!=0xff || frame[len(frame)-1]!=0xd9 {
   return errors.New("invalid JPEG start/end markers")
  }
  if err:=part.Close();err!=nil{return err}
  r.publish(frame)
 }
 return ctx.Err()
}

func (r *Relay) publish(frame []byte) {
 r.mu.Lock();defer r.mu.Unlock()
 r.received++;r.lastFrame=time.Now()
 for _,sub:=range r.subscribers {
  ch:=sub.frames
  select {
  case ch<-frame:
  default:
   // Latest frame only, one reference per viewer, no producer backpressure.
   select { case <-ch: default: }
   ch<-frame
  }
 }
}

func (r *Relay) flashRequestingViewersLocked() int {
 count:=0
 for _,sub:=range r.subscribers {
  if sub.flashRequested { count++ }
 }
 return count
}

func (r *Relay) recomputeFlashDesiredLocked() {
 switch r.flashMode {
 case FlashModeOn:
  r.flashDesired=!r.closed
 case FlashModeOff:
  r.flashDesired=false
 default:
  r.flashDesired=!r.closed && r.flashRequestingViewersLocked()>0
 }
}

func (r *Relay) subscribe(flashRequested bool)(uint64,<-chan []byte,bool){
 r.mu.Lock()
 if r.closed||len(r.subscribers)>=r.cfg.MaxClients{
  r.mu.Unlock()
  return 0,nil,false
 }
 r.nextID++
 id:=r.nextID
 ch:=make(chan []byte,1)
 r.subscribers[id]=subscriber{frames:ch,flashRequested:flashRequested}
 r.recomputeFlashDesiredLocked()
 r.mu.Unlock()
 r.signalFlash()
 return id,ch,true
}

func (r *Relay) unsubscribe(id uint64){
 r.mu.Lock()
 if sub,ok:=r.subscribers[id];ok{
  delete(r.subscribers,id)
  close(sub.frames)
 }
 r.recomputeFlashDesiredLocked()
 r.mu.Unlock()
 r.signalFlash()
}

func (r *Relay) signalFlash(){
 if r.cfg.FlashControlURL==""{return}
 select{case r.flashWake<-struct{}{}:default:}
}

func parseFlashRequest(req *http.Request)(bool,error){
 value:=strings.ToLower(strings.TrimSpace(req.URL.Query().Get("flash")))
 switch value {
 case "", "0", "false", "off", "no":
  return false,nil
 case "1", "true", "on", "yes":
  return true,nil
 default:
  return false,errors.New("flash query must be 0/1 or false/true")
 }
}

func (r *Relay) setFlashMode(mode string) error {
 mode=strings.ToLower(strings.TrimSpace(mode))
 if mode!=FlashModeAuto && mode!=FlashModeOn && mode!=FlashModeOff {
  return errors.New("flash state must be auto, on or off")
 }
 if r.cfg.FlashControlURL=="" {
  return errors.New("camera flash control is disabled")
 }
 r.mu.Lock()
 r.flashMode=mode
 r.flashAppliedKnown=false
 r.recomputeFlashDesiredLocked()
 r.mu.Unlock()
 r.signalFlash()
 return nil
}

func (r *Relay) setFlash(ctx context.Context,on bool)error{
 if r.cfg.FlashControlURL==""{return nil}
 body:="{\"on\":false}"
 if on{body="{\"on\":true}"}
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,r.cfg.FlashControlURL,strings.NewReader(body))
 if err!=nil{return err}
 req.Header.Set("Content-Type","application/json")
 resp,err:=r.flashClient.Do(req)
 if err!=nil{return err}
 defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{
  _,_=io.Copy(io.Discard,io.LimitReader(resp.Body,1024))
  return fmt.Errorf("camera flash HTTP %d",resp.StatusCode)
 }
 return nil
}

func (r *Relay) reconcileFlash(ctx context.Context) error {
 if r.cfg.FlashControlURL==""{return nil}
 r.flashMu.Lock()
 defer r.flashMu.Unlock()

 r.mu.Lock()
 desired,applied,known:=r.flashDesired,r.flashApplied,r.flashAppliedKnown
 r.mu.Unlock()
 if known && desired==applied{return nil}

 err:=r.setFlash(ctx,desired)
 r.mu.Lock()
 if err!=nil{
  r.flashAppliedKnown=false
  r.flashError=err.Error()
 }else{
  r.flashApplied=desired
  r.flashAppliedKnown=true
  r.flashError=""
 }
 newestDesired:=r.flashDesired
 r.mu.Unlock()
 if err!=nil{
  r.log.Warn("camera flash reconcile failed","desired",desired,"error",err)
  return err
 }
 if newestDesired!=desired {
  r.signalFlash()
 }
 return nil
}

func (r *Relay) flashLoop(ctx context.Context){
 ticker:=time.NewTicker(r.cfg.ReconnectDelay)
 defer ticker.Stop()
 for{
  select{
  case <-ctx.Done():return
  case <-r.flashWake:_=r.reconcileFlash(ctx)
  case <-ticker.C:_=r.reconcileFlash(ctx)
  }
 }
}

// Handler serves health, bounded streams and optional show-LAN flash override.
func(r *Relay) Handler()http.Handler{
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /api/v0/health",r.health)
 mux.HandleFunc("GET /api/v0/stream",r.stream)
 mux.HandleFunc("POST /api/v0/flash",r.flashControl)
 return mux
}

func(r *Relay) health(w http.ResponseWriter,_ *http.Request){
 r.mu.Lock()
 up,last,received,viewers:=r.connected,r.lastFrame,r.received,len(r.subscribers)
 flashRequesting:=r.flashRequestingViewersLocked()
 flashMode:=r.flashMode
 flashDesired,flashApplied,flashKnown,flashError:=r.flashDesired,r.flashApplied,r.flashAppliedKnown,r.flashError
 r.mu.Unlock()
 age:=int64(-1)
 if !last.IsZero(){age=time.Since(last).Milliseconds()}
 state:="reconnecting"
 code:=http.StatusServiceUnavailable
 if up && age>=0 && age<r.cfg.FrameTimeout.Milliseconds(){state="ready";code=http.StatusOK}
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 w.WriteHeader(code)
 _=json.NewEncoder(w).Encode(map[string]any{
  "state":state,"upstream_connected":up,"frames_received":received,
  "viewers":viewers,"last_frame_age_ms":age,"max_clients":r.cfg.MaxClients,
  "flash_control_enabled":r.cfg.FlashControlURL!="",
  // Backward-compatible keys retained for existing health tooling.
  "flash_desired":flashDesired,"flash_applied":flashKnown&&flashApplied,"flash_error":flashError,
  // Selective/manual flash state.
  "flash_mode":flashMode,
  "flash_requesting_viewers":flashRequesting,
  "flash_desired_on":flashDesired,
  "flash_applied_known":flashKnown,
  "flash_applied_on":flashKnown&&flashApplied,
  "flash_last_error":flashError,
 })
}

func(r *Relay) flashControl(w http.ResponseWriter,req *http.Request){
 if r.cfg.FlashControlURL==""{
  http.Error(w,"camera flash control disabled",http.StatusServiceUnavailable)
  return
 }
 state:=strings.ToLower(strings.TrimSpace(req.URL.Query().Get("state")))
 if err:=r.setFlashMode(state);err!=nil{
  http.Error(w,err.Error(),http.StatusBadRequest)
  return
 }
 err:=r.reconcileFlash(req.Context())
 r.mu.Lock()
 mode,known,applied,lastErr:=r.flashMode,r.flashAppliedKnown,r.flashApplied,r.flashError
 r.mu.Unlock()
 code:=http.StatusOK
 if err!=nil{code=http.StatusBadGateway}
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 w.WriteHeader(code)
 _=json.NewEncoder(w).Encode(map[string]any{
  "flash_mode":mode,
  "flash_applied_known":known,
  "flash_applied_on":known&&applied,
  "flash_last_error":lastErr,
 })
}

func(r *Relay) stream(w http.ResponseWriter,req *http.Request){
 flashRequested,err:=parseFlashRequest(req)
 if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
 if flashRequested && r.cfg.FlashControlURL==""{
  http.Error(w,"camera flash control unavailable",http.StatusServiceUnavailable)
  return
 }
 id,frames,ok:=r.subscribe(flashRequested)
 if !ok{http.Error(w,"relay viewer limit reached",http.StatusServiceUnavailable);return}
 defer r.unsubscribe(id)
 w.Header().Set("Content-Type","multipart/x-mixed-replace;boundary="+boundary)
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 ctl:=http.NewResponseController(w)
 idle:=time.NewTimer(2*r.cfg.FrameTimeout)
 defer idle.Stop()
 for {
  select {
  case <-req.Context().Done():return
  case <-idle.C:return // release viewer slot if source stopped producing frames
  case frame,open:=<-frames:
   if !open{return}
   if err:=ctl.SetWriteDeadline(time.Now().Add(r.cfg.WriteTimeout));err!=nil{return}
   if _,err:=fmt.Fprintf(w,"--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n",boundary,len(frame));err!=nil{return}
   if _,err:=w.Write(frame);err!=nil{return}
   if _,err:=io.WriteString(w,"\r\n");err!=nil{return}
   if err:=ctl.Flush();err!=nil{return}
   if !idle.Stop(){select{case <-idle.C:default:}}
   idle.Reset(2*r.cfg.FrameTimeout)
  }
 }
}
