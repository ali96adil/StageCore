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

type Config struct {
 SourceURL string
 FlashURL string
 MaxClients int
 MaxFrameBytes int64
 ReconnectDelay time.Duration
 HeaderTimeout time.Duration
 FrameTimeout time.Duration
 WriteTimeout time.Duration
 FlashTimeout time.Duration
}

type subscriber struct {
 frames chan []byte
 flashRequested bool
}

type Relay struct {
 cfg Config
 log *slog.Logger
 client *http.Client
 controlClient *http.Client
 started atomic.Bool
 mu sync.Mutex
 subscribers map[uint64]subscriber
 nextID uint64
 closed bool
 connected bool
 received uint64
 lastFrame time.Time

 // flashMu serializes desired/applied reconciliation and manual override mode.
 // AUTO follows only viewers that explicitly requested flash. ON/OFF override it.
 flashMu sync.Mutex
 flashMode string
 flashKnown bool
 flashApplied bool
 flashError string
}

func FlashURLForSource(source string) (string, error) {
 u, err := url.Parse(source)
 if err != nil || u == nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
  return "", errors.New("source must be HTTP with a host and no credentials or fragment")
 }
 return (&url.URL{
  Scheme: "http",
  Host: net.JoinHostPort(u.Hostname(), "80"),
  Path: "/api/v0/flash",
 }).String(), nil
}

func New(cfg Config, log *slog.Logger) (*Relay, error) {
 u, err := url.Parse(cfg.SourceURL)
 if err != nil || u == nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
  return nil, errors.New("source must be HTTP with a host and no credentials or fragment")
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
    cfg.FlashTimeout < 250*time.Millisecond || cfg.FlashTimeout > 10*time.Second {
  return nil, errors.New("relay resource limit is out of bounds")
 }

 var flashURL *url.URL
 if strings.TrimSpace(cfg.FlashURL) != "" {
  flashURL, err = url.Parse(cfg.FlashURL)
  if err != nil || flashURL == nil || flashURL.Scheme != "http" || flashURL.Hostname() == "" ||
     flashURL.User != nil || flashURL.Fragment != "" || flashURL.RawQuery != "" ||
     flashURL.Path != "/api/v0/flash" {
   return nil, errors.New("flash URL must be HTTP /api/v0/flash with a host and no credentials, query or fragment")
  }
  if !strings.EqualFold(flashURL.Hostname(), u.Hostname()) {
   return nil, errors.New("flash URL host must match camera source host")
  }
 }

 if log == nil { log = slog.Default() }
 transport := &http.Transport{
  Proxy: nil,
  DialContext: sourceDialContext,
  DisableCompression:true, DisableKeepAlives:true, MaxConnsPerHost:1,
  ResponseHeaderTimeout:cfg.HeaderTimeout,
 }
 client := &http.Client{Transport:transport, CheckRedirect:func(*http.Request,[]*http.Request)error{
  return errors.New("camera redirects are not allowed")
 }}

 var controlClient *http.Client
 if flashURL != nil {
  controlTransport := &http.Transport{
   Proxy:nil, DialContext:sourceDialContext, DisableCompression:true,
   DisableKeepAlives:true, MaxConnsPerHost:2, ResponseHeaderTimeout:cfg.FlashTimeout,
  }
  controlClient=&http.Client{
   Transport:controlTransport,
   Timeout:cfg.FlashTimeout,
   CheckRedirect:func(*http.Request,[]*http.Request)error{return errors.New("camera redirects are not allowed")},
  }
 }

 return &Relay{
  cfg:cfg, log:log, client:client, controlClient:controlClient,
  subscribers:make(map[uint64]subscriber),
  flashMode:FlashModeAuto,
 }, nil
}

func (r *Relay) flashRequestingViewersLocked() int {
 count:=0
 for _,sub:=range r.subscribers {
  if sub.flashRequested { count++ }
 }
 return count
}

func (r *Relay) desiredFlashLocked() bool {
 switch r.flashMode {
 case FlashModeOn:
  return true
 case FlashModeOff:
  return false
 default:
  r.mu.Lock()
  desired:=r.flashRequestingViewersLocked()>0 && !r.closed
  r.mu.Unlock()
  return desired
 }
}

func (r *Relay) reconcileFlash() {
 if r.controlClient == nil || strings.TrimSpace(r.cfg.FlashURL) == "" { return }
 r.flashMu.Lock()
 defer r.flashMu.Unlock()

 for {
  desired:=r.desiredFlashLocked()
  if r.flashKnown && r.flashApplied == desired {
   r.flashError = ""
   return
  }

  state := FlashModeOff
  if desired { state = FlashModeOn }
  req, err := http.NewRequest(http.MethodPost, r.cfg.FlashURL+"?state="+state, nil)
  if err == nil {
   var resp *http.Response
   resp, err = r.controlClient.Do(req)
   if resp != nil {
    _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
    _ = resp.Body.Close()
    if err == nil && resp.StatusCode != http.StatusOK {
     err = fmt.Errorf("camera flash HTTP %d", resp.StatusCode)
    }
   }
  }
  if err != nil {
   r.flashKnown = false
   r.flashError = err.Error()
   r.log.Warn("camera flash control failed", "mode", r.flashMode, "desired_on", desired, "error", err)
   return
  }

  r.flashKnown = true
  r.flashApplied = desired
  r.flashError = ""

  if r.desiredFlashLocked() == desired { return }
  // Viewer requests or manual mode changed while the bounded camera request
  // was in flight. Reconcile the newest desired state before returning.
 }
}

func (r *Relay) setFlashMode(mode string) error {
 mode=strings.ToLower(strings.TrimSpace(mode))
 if mode!=FlashModeAuto && mode!=FlashModeOn && mode!=FlashModeOff {
  return errors.New("flash state must be auto, on or off")
 }
 if r.controlClient == nil {
  return errors.New("camera flash control is disabled")
 }
 r.flashMu.Lock()
 r.flashMode=mode
 r.flashKnown=false
 r.flashMu.Unlock()
 r.reconcileFlash()
 return nil
}

// Run is the sole upstream owner. Subscriber connections never contact the camera.
func (r *Relay) Run(ctx context.Context) error {
 if !r.started.CompareAndSwap(false,true) { return errors.New("relay already started") }
 // Startup always begins AUTO with zero viewers, which reconciles camera flash OFF.
 r.reconcileFlash()
 defer func(){
  r.mu.Lock()
  r.connected=false
  r.closed=true
  for id,sub := range r.subscribers {
   delete(r.subscribers,id)
   close(sub.frames)
  }
  r.mu.Unlock()

  // Shutdown must fail safe to OFF regardless of a manual ON override.
  r.flashMu.Lock()
  r.flashMode=FlashModeOff
  r.flashKnown=false
  r.flashMu.Unlock()
  r.reconcileFlash()

  r.client.CloseIdleConnections()
  if r.controlClient != nil { r.controlClient.CloseIdleConnections() }
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

func (r *Relay) subscribe(flashRequested bool)(uint64,<-chan []byte,bool){
 r.mu.Lock();defer r.mu.Unlock()
 if r.closed||len(r.subscribers)>=r.cfg.MaxClients{return 0,nil,false}
 r.nextID++
 id:=r.nextID
 ch:=make(chan []byte,1)
 r.subscribers[id]=subscriber{frames:ch,flashRequested:flashRequested}
 return id,ch,true
}

func (r *Relay) unsubscribe(id uint64){
 r.mu.Lock()
 if sub,ok:=r.subscribers[id];ok{
  delete(r.subscribers,id)
  close(sub.frames)
 }
 r.mu.Unlock()
 r.reconcileFlash()
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

// Handler serves health, bounded streams, and optional trusted-show-LAN flash control.
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
 r.mu.Unlock()

 r.flashMu.Lock()
 flashEnabled:=r.controlClient!=nil
 flashMode,flashKnown,flashApplied,flashError:=r.flashMode,r.flashKnown,r.flashApplied,r.flashError
 desired:=false
 switch flashMode {
 case FlashModeOn:
  desired=true
 case FlashModeAuto:
  desired=flashRequesting>0
 }
 r.flashMu.Unlock()

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
  "flash_control_enabled":flashEnabled,
  "flash_mode":flashMode,
  "flash_requesting_viewers":flashRequesting,
  "flash_desired_on":flashEnabled && desired,
  "flash_applied_known":flashKnown,
  "flash_applied_on":flashKnown && flashApplied,
  "flash_last_error":flashError,
 })
}

func(r *Relay) flashControl(w http.ResponseWriter,req *http.Request){
 if r.controlClient==nil {
  http.Error(w,"camera flash control disabled",http.StatusServiceUnavailable)
  return
 }
 state:=strings.ToLower(strings.TrimSpace(req.URL.Query().Get("state")))
 if err:=r.setFlashMode(state);err!=nil{
  http.Error(w,err.Error(),http.StatusBadRequest)
  return
 }

 r.flashMu.Lock()
 mode,known,applied,lastErr:=r.flashMode,r.flashKnown,r.flashApplied,r.flashError
 r.flashMu.Unlock()
 code:=http.StatusOK
 if lastErr!="" { code=http.StatusBadGateway }
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
 if err!=nil{
  http.Error(w,err.Error(),http.StatusBadRequest)
  return
 }
 if flashRequested && r.controlClient==nil{
  http.Error(w,"camera flash control unavailable",http.StatusServiceUnavailable)
  return
 }

 id,frames,ok:=r.subscribe(flashRequested)
 if !ok{http.Error(w,"relay viewer limit reached",http.StatusServiceUnavailable);return}
 r.reconcileFlash()
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
  case <-idle.C:return
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
