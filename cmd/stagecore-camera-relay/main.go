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
 var source,listen string
 var allowLAN bool
 flag.StringVar(&source,"source","","camera MJPEG HTTP URL (required)")
 flag.StringVar(&listen,"listen","127.0.0.1:9081","relay HTTP listen address (local-only default)")
 flag.BoolVar(&allowLAN,"allow-lan",false,"explicitly permit unauthenticated LAN HTTP; use show-network isolation")
 flag.Parse()
 if source=="" || (!localOnly(listen)&&!allowLAN) {
  fmt.Fprintln(os.Stderr,"required: -source; non-loopback -listen requires -allow-lan")
  os.Exit(2)
 }
 logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
 relay,err:=camerarelay.New(camerarelay.Config{SourceURL:source},logger)
 if err!=nil {logger.Error("invalid relay configuration","error",err);os.Exit(2)}
 listener,err:=net.Listen("tcp",listen)
 if err!=nil{logger.Error("cannot listen","error",err);os.Exit(1)}
 defer listener.Close()
 if !localOnly(listen){logger.Warn("LAN relay HTTP is unauthenticated; restrict network and firewall access")}
 server:=&http.Server{
  Handler:relay.Handler(),ReadHeaderTimeout:5*time.Second,IdleTimeout:10*time.Second,
  // Each streaming handler imposes per-frame write deadlines.
 }
 ctx,stop:=signal.NotifyContext(context.Background(),syscall.SIGINT,syscall.SIGTERM)
 defer stop()
 errs:=make(chan error,2)
 go func(){errs<-relay.Run(ctx)}()
 go func(){errs<-server.Serve(listener)}()
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
