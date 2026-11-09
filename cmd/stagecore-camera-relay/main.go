// stagecore-camera-relay is a standalone show-LAN experiment, not the Hub.
package main

import (
 "context"
 "errors"
 "flag"
 "fmt"
 "log/slog"
 "net"
 "net/http"
 "os"
 "os/signal"
 "strings"
 "syscall"
 "time"

 "github.com/ali96adil/StageCore/internal/camerarelay"
)

func localOnly(address string) bool {
 host,_,err:=net.SplitHostPort(address)
 if err!=nil{return false}
 if strings.EqualFold(host,"localhost"){return true}
 ip:=net.ParseIP(host)
 return ip!=nil && ip.IsLoopback()
}

func main() {
 var source,listen,flashControl string
 var viewerTokenFile,controlTokenFile,tlsCert,tlsKey string
 var allowLAN bool
 flag.StringVar(&source,"source","","camera MJPEG HTTP URL (required)")
 flag.StringVar(&listen,"listen","127.0.0.1:9081","relay HTTP listen address (local-only default)")
 flag.StringVar(&flashControl,"flash-control","","optional camera flash control HTTP URL")
 flag.BoolVar(&allowLAN,"allow-lan",false,"explicitly permit unauthenticated LAN HTTP; use show-network isolation")
 flag.StringVar(&viewerTokenFile,"viewer-token-file","","owner-only file with the camera viewer bearer token")
 flag.StringVar(&controlTokenFile,"control-token-file","","owner-only file with the camera control bearer token")
 flag.StringVar(&tlsCert,"tls-cert","","TLS certificate path")
 flag.StringVar(&tlsKey,"tls-key","","TLS private key path")
 flag.Parse()
 qualified:=viewerTokenFile!="" || controlTokenFile!=""
 if (tlsCert=="")!=(tlsKey=="") || (qualified && (viewerTokenFile=="" || controlTokenFile=="" || (!localOnly(listen) && tlsCert==""))){
  fmt.Fprintln(os.Stderr,"camera authorization requires both token files and TLS on non-loopback listeners; TLS requires both certificate and key")
  os.Exit(2)
 }
 if source=="" || (!localOnly(listen)&&!allowLAN&&!qualified) {
  fmt.Fprintln(os.Stderr,"required: -source; non-loopback -listen requires -allow-lan")
  os.Exit(2)
 }
 logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
 cfg:=camerarelay.Config{SourceURL:source,FlashControlURL:flashControl}
 if qualified {
  authorize,err:=camerarelay.FileAccess(viewerTokenFile,controlTokenFile)
  if err!=nil {logger.Error("invalid relay authorization","error",err);os.Exit(2)}
  cfg.Authorize=authorize
 }
 relay,err:=camerarelay.New(cfg,logger)
 if err!=nil {logger.Error("invalid relay configuration","error",err);os.Exit(2)}
 listener,err:=net.Listen("tcp",listen)
 if err!=nil{logger.Error("cannot listen","error",err);os.Exit(1)}
 defer listener.Close()
 if !localOnly(listen)&&!qualified{logger.Warn("LAN relay HTTP is unauthenticated; restrict network and firewall access")}
 server:=&http.Server{
  Handler:relay.Handler(),ReadHeaderTimeout:5*time.Second,IdleTimeout:10*time.Second,
  // Each streaming handler imposes per-frame write deadlines.
 }
 ctx,stop:=signal.NotifyContext(context.Background(),syscall.SIGINT,syscall.SIGTERM)
 defer stop()
 errs:=make(chan error,2)
 go func(){errs<-relay.Run(ctx)}()
 go func(){
  if tlsCert!=""{errs<-server.ServeTLS(listener,tlsCert,tlsKey);return}
  errs<-server.Serve(listener)
 }()
 logger.Info("camera relay started","listen",listener.Addr().String())
 select {
 case err:=<-errs:
  if err!=nil && !errors.Is(err,http.ErrServerClosed) {
   logger.Error("relay stopped unexpectedly","error",err)
  }
 case <-ctx.Done():
 }
 stop()
 shutdownCtx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
 defer cancel()
 if err:=server.Shutdown(shutdownCtx);err!=nil{
  logger.Warn("HTTP shutdown incomplete","error",err)
 }
}
