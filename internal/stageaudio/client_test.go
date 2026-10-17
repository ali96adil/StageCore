package stageaudio

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalClientOnlyUsesExplicitUnixSocket(t *testing.T) {
	dir:=t.TempDir()
	for _,value:=range []string{"","http://localhost:1234","relative.sock"} {
		if _,err:=NewLocalClient(value);err==nil{t.Fatalf("invalid socket config accepted: %q",value)}
	}
	path:=filepath.Join(dir,"stageaudio.sock")
	server,err:=net.Listen("unix",path)
	if err!=nil{t.Fatal(err)}
	defer server.Close()
	if err:=os.Chmod(path,0600);err!=nil{t.Fatal(err)}
	client,err:=NewLocalClient(path)
	if err!=nil{t.Fatal(err)}
	go func(){
		conn,acceptErr:=server.Accept()
		if acceptErr!=nil{return}
		defer conn.Close()
		var req Request
		if json.NewDecoder(bufio.NewReader(conn)).Decode(&req)!=nil{return}
		_ = json.NewEncoder(conn).Encode(LocalReply{Status:Status{Note:"LOCAL_COMMAND_ACK"},Media:[]string{"wind.wav"}})
	}()
	ctx,cancel:=context.WithTimeout(context.Background(),time.Second*2)
	defer cancel()
	reply,err:=client.Call(ctx,Request{Operation:"FILES"})
	if err!=nil||len(reply.Media)!=1||reply.Media[0]!="wind.wav" {
		t.Fatalf("unexpected local reply: %+v err=%v",reply,err)
	}
}
func TestLocalClientRejectsSymlinkAndOrdinaryFile(t *testing.T) {
	root:=t.TempDir()
	file:=filepath.Join(root,"ordinary.sock")
	if err:=os.WriteFile(file,[]byte("unsafe"),0600);err!=nil{t.Fatal(err)}
	link:=filepath.Join(root,"alias.sock")
	if err:=os.Symlink(file,link);err!=nil{t.Fatal(err)}
	for _,path:=range []string{file,link}{
		client,err:=NewLocalClient(path)
		if err!=nil{t.Fatal(err)}
		ctx,cancel:=context.WithTimeout(context.Background(),time.Second)
		_,err=client.Call(ctx,Request{Operation:"STATUS"})
		cancel()
		if err==nil{t.Fatalf("unsafe socket path accepted %q",path)}
	}
}
