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
 subscribers map[uint64]chan []byte
 nextID uint64
 closed bool
 connected bool
 received uint64
 lastFrame time.Time
 flashWake chan struct{}
 flashDesired bool
 flashApplied bool
 flashError string
}

func New(cfg Config, log *slog.Logger) (*Relay, error) {
 u, err := url.Parse(cfg.SourceURL)
 if err != nil || u == nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
  return nil, errors.New("source must be HTTP with a host and no credentials or fragment")
 }
 if cfg.FlashControlURL != "" {
  flashURL, flashErr := url.Parse(cfg.FlashControlURL)
  if flashErr != nil || flashURL == nil || flashURL.Scheme != "http" || flashURL.Hostname() == "" || flashURL.User != nil || flashURL.Fragment != "" {
   return nil, errors.New("flash control must be HTTP with a host and no credentials or fragment")
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
  subscribers:make(map[uint64]chan []byte), flashWake:make(chan struct{},1),
 }, nil
}

// Run is the sole upstream owner. Subscriber connections never contact the camera.
func (r *Relay) Run(ctx context.Context) error {
 if !r.started.CompareAndSwap(false,true) { return errors.New("relay already started") }
 if r.cfg.FlashControlURL != "" {
  go r.flashLoop(ctx)
 }
 defer func(){
  r.mu.Lock()
  r.connected=false; r.closed=true; r.flashDesired=false
  for id,ch := range r.subscribers { delete(r.subscribers,id); close(ch) }
  r.mu.Unlock()
  if r.cfg.FlashControlURL != "" {
   offCtx,cancel:=context.WithTimeout(context.Background(),r.cfg.FlashTimeout)
   if err:=r.setFlash(offCtx,false);err!=nil{
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
 for _,ch:=range r.subscribers {
  select {
  case ch<-frame:
  default:
   // Latest frame only, one reference per viewer, no producer backpressure.
   select { case <-ch: default: }
   ch<-frame
  }
 }
}

func (r *Relay) subscribe()(uint64,<-chan []byte,bool){
 r.mu.Lock();defer r.mu.Unlock()
 if r.closed||len(r.subscribers)>=r.cfg.MaxClients{return 0,nil,false}
 r.nextID++
 id:=r.nextID
 ch:=make(chan []byte,1)
 r.subscribers[id]=ch
 r.flashDesired=true
 r.signalFlashLocked()
 return id,ch,true
}
func (r *Relay) unsubscribe(id uint64){
 r.mu.Lock();defer r.mu.Unlock()
 if ch,ok:=r.subscribers[id];ok{
  delete(r.subscribers,id);close(ch)
  r.flashDesired=len(r.subscribers)>0
  r.signalFlashLocked()
 }
}

func (r *Relay) signalFlashLocked(){
 if r.cfg.FlashControlURL==""{return}
 select{case r.flashWake<-struct{}{}:default:}
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

func (r *Relay) reconcileFlash(ctx context.Context){
 if r.cfg.FlashControlURL==""{return}
 r.mu.Lock();desired,applied:=r.flashDesired,r.flashApplied;r.mu.Unlock()
 if desired==applied{return}
 err:=r.setFlash(ctx,desired)
 r.mu.Lock()
 if err!=nil{
  r.flashError=err.Error()
 }else{
  r.flashApplied=desired
  r.flashError=""
 }
 r.mu.Unlock()
 if err!=nil{
  r.log.Warn("camera flash reconcile failed","desired",desired,"error",err)
 }
}

func (r *Relay) flashLoop(ctx context.Context){
 ticker:=time.NewTicker(r.cfg.ReconnectDelay)
 defer ticker.Stop()
 for{
  select{
  case <-ctx.Done():return
  case <-r.flashWake:r.reconcileFlash(ctx)
  case <-ticker.C:r.reconcileFlash(ctx)
  }
 }
}

// Handler serves read-only health and a bounded number of JPEG stream viewers.
func(r *Relay) Handler()http.Handler{
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /api/v0/health",r.health)
 mux.HandleFunc("GET /api/v0/stream",r.stream)
 return mux
}

func(r *Relay) health(w http.ResponseWriter,_ *http.Request){
 r.mu.Lock()
 up,last,received,viewers:=r.connected,r.lastFrame,r.received,len(r.subscribers)
 flashDesired,flashApplied,flashError:=r.flashDesired,r.flashApplied,r.flashError
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
  "flash_desired":flashDesired,"flash_applied":flashApplied,"flash_error":flashError,
 })
}

func(r *Relay) stream(w http.ResponseWriter,req *http.Request){
 id,frames,ok:=r.subscribe()
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
