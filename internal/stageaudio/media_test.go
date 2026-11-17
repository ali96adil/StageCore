package stageaudio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMediaFilesLimitsToSafeLocalAudio(t *testing.T) {
	dir := t.TempDir()
	valid := []string{"music.mp3", "wind.wav", "cue.flac"}
	for _, name := range append(valid, "not-audio.exe", ".hidden.mp3") {
		if err := os.WriteFile(filepath.Join(dir,name),[]byte("fixture"),0600);err != nil {t.Fatal(err)}
	}
	outside:=filepath.Join(t.TempDir(),"outside.wav")
	if err:=os.WriteFile(outside,[]byte("private"),0600);err!=nil{t.Fatal(err)}
	if err:=os.Symlink(outside,filepath.Join(dir,"link.wav"));err!=nil{t.Fatal(err)}
	if err:=os.Mkdir(filepath.Join(dir,"folder.mp3"),0700);err!=nil{t.Fatal(err)}
	c,err:=NewController(dir,&fakeSender{})
	if err!=nil{t.Fatal(err)}
	list,err:=c.MediaFiles()
	if err!=nil{t.Fatal(err)}
	if len(list)!=3{t.Fatalf("unexpected media list: %+v",list)}
	for _,name:=range list {
		if name!="music.mp3" && name!="wind.wav" && name!="cue.flac" {
			t.Fatalf("unsafe media exposed to phone: %q",name)
		}
	}
}
