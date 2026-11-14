package stageaudio

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type bluezFake struct{
	paired string
	connected bool
	calls [][]string
	failConnect bool
}
func (f *bluezFake) Run(_ context.Context,args ...string)(string,error){
	f.calls=append(f.calls,append([]string(nil),args...))
	if len(args)==2&&args[0]=="devices"&&args[1]=="Paired"{return f.paired,nil}
	if len(args)==2&&args[0]=="info" {
		connected:="no"
		if f.connected {connected="yes"}
		return "Device AA:BB:CC:DD:EE:FF TEST SPEAKER\n"+
			" Name: Stage Speaker\n Paired: yes\n Connected: "+connected+"\n"+
			" UUID: Audio Sink (0000110b-0000-1000-8000-00805f9b34fb)\n",nil
	}
	if len(args)==3&&args[0]=="connect"{
		if f.failConnect{return "",errors.New("offline")}
		f.connected=true
		return "Connection successful",nil
	}
	return "",errors.New("unhandled command")
}
func TestBluetoothOnlyListsPairedAudioSinks(t *testing.T){
	f:=&bluezFake{paired:"Device AA:BB:CC:DD:EE:FF Stage Speaker\n"}
	b:=&BluetoothService{runner:f}
	list,err:=b.PairedSpeakers(context.Background())
	if err!=nil||len(list)!=1||list[0].Address!="AA:BB:CC:DD:EE:FF"||!list[0].Paired||!list[0].AudioSink{
		t.Fatalf("paired speaker not found: %+v %v",list,err)
	}
	if list[0].Connected {t.Fatal("must not claim unconnected Bluetooth ready")}
	for _,call:=range f.calls{if call[0]=="pair"||call[0]=="trust"{t.Fatal("discovery changed Bluetooth trust")}}
}
func TestBluetoothConnectRequiresValidatedPairedAudioSink(t *testing.T){
	f:=&bluezFake{paired:"Device AA:BB:CC:DD:EE:FF Stage Speaker\n"}
	b:=&BluetoothService{runner:f}
	for _,address:=range []string{"01:02", "AA:BB:CC:DD:EE:FF;touch /tmp/exploit", "../other"}{
		if _,err:=b.ConnectPairedSpeaker(context.Background(),address);!errors.Is(err,ErrInvalidCommand){
			t.Fatalf("accepted malformed address %q: %v",address,err)
		}
	}
	if len(f.calls)!=0{t.Fatalf("bad address was given to bluetoothctl: %+v",f.calls)}
	out,err:=b.ConnectPairedSpeaker(context.Background(),"aa:bb:cc:dd:ee:ff")
	if err!=nil||!out.Connected {t.Fatalf("paired connect not confirmed: %+v %v",out,err)}
	var sawConnect bool
	for _,call:=range f.calls{
		if call[0]=="connect"{
			sawConnect=true
			if strings.Join(call," ")!="connect AA:BB:CC:DD:EE:FF a2dp-sink"{
				t.Fatalf("connect must select A2DP and use fixed args: %+v",call)
			}
		}
	}
	if !sawConnect{t.Fatal("no connect to paired speaker")}
	// A connected speaker must not be reconnected automatically.
	_,err=b.ConnectPairedSpeaker(context.Background(),out.Address)
	if err!=nil{t.Fatal(err)}
}
func TestBluetoothConnectionFailureReportedWithoutSpeakerSelection(t *testing.T){
	f:=&bluezFake{failConnect:true}
	b:=&BluetoothService{runner:f}
	_,err:=b.ConnectPairedSpeaker(context.Background(),"AA:BB:CC:DD:EE:FF")
	if !errors.Is(err,ErrBackend){t.Fatalf("connect failure must not look READY: %v",err)}
}
