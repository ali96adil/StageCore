package stageaudio

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type outputSender struct {
	calls []sentCommand
	queryResult json.RawMessage
	failB bool
}
func (f *outputSender) Send(_ context.Context, deck string, command []any) error {
	f.calls=append(f.calls,sentCommand{Deck:deck, Args:command})
	if f.failB && deck==DeckB { return errors.New("deck B output unavailable") }
	return nil
}
func (f *outputSender) Query(_ context.Context, deck string, command []any) (json.RawMessage,error) {
	if deck!=DeckA || len(command)!=2 || command[0]!="get_property" || command[1]!="audio-device-list" {
		return nil,ErrInvalidCommand
	}
	return f.queryResult,nil
}
func routeFixture(t *testing.T) (*Controller,*outputSender) {
	t.Helper()
	f:=&outputSender{queryResult:json.RawMessage(`[
	  {"name":"auto","description":"System default"},
	  {"name":"pulse/alsa_output.platform-bcm2835_audio.analog-stereo","description":"Pi 4 headphones"},
	  {"name":"pulse/bluez_output.AA_BB_CC_DD_EE_FF.a2dp-sink","description":"Bluetooth stereo speaker"},
	  {"name":"alsa/usb_DAC","description":"USB DAC"}
	]`)}
	c,err:=NewController(t.TempDir(),f)
	if err!=nil{t.Fatal(err)}
	return c,f
}
func TestOutputDiscoveryRecognizesAnalogAndBluetooth(t *testing.T){
	c,_:=routeFixture(t)
	items,err:=c.Outputs(context.Background())
	if err!=nil{t.Fatal(err)}
	if len(items)!=3 || items[0].Kind!="ANALOG" || items[1].Kind!="BLUETOOTH" || items[2].Kind!="USB"{
		t.Fatalf("unexpected audio output list: %+v",items)
	}
}
func TestExplicitSwitchIsDeckPairedAndRejectsUnknownSink(t *testing.T){
	c,f:=routeFixture(t)
	id:="pulse/bluez_output.AA_BB_CC_DD_EE_FF.a2dp-sink"
	st,err:=c.SelectOutput(context.Background(),id)
	if err!=nil || st.OutputID!=id || st.OutputError!="" || len(f.calls)!=2{
		t.Fatalf("select A2DP failed status=%+v err=%v calls=%+v",st,err,f.calls)
	}
	for _,call:=range f.calls{
		if call.Args[0]!="set_property" || call.Args[1]!="audio-device" || call.Args[2]!=id {
			t.Fatalf("wrong routing request: %+v",call)
		}
	}
	if _,err:=c.SelectOutput(context.Background(),"pulse/invented_output");!errors.Is(err,ErrInvalidCommand){
		t.Fatalf("invented output accepted: %v",err)
	}
	if len(f.calls)!=2{t.Fatal("invalid output reached mpv")}
	command(t,c,Request{Deck:DeckA,Operation:"VOLUME",Volume:volumePointer(45)})
	// Keep a deck marked PAUSED with no media I/O dependency.
	c.mu.Lock()
	deck:=c.decks[DeckA]; deck.Commanded="PAUSED"; c.decks[DeckA]=deck
	c.mu.Unlock()
	if c.OutputChangeAllowed(){t.Fatal("PAUSED deck may not change sink")}
	if _,err:=c.SelectOutput(context.Background(),itemsAnalog());!errors.Is(err,ErrInvalidCommand){
		t.Fatalf("switch with PAUSED deck allowed: %v",err)
	}
}
func itemsAnalog() string { return "pulse/alsa_output.platform-bcm2835_audio.analog-stereo" }

func volumePointer(v float64)*float64{return &v}

func TestOutputPartialSwitchIsNeverReportedAsComplete(t *testing.T){
	c,f:=routeFixture(t)
	f.failB=true
	st,err:=c.SelectOutput(context.Background(),"pulse/bluez_output.AA_BB_CC_DD_EE_FF.a2dp-sink")
	if !errors.Is(err,ErrBackend) || st.OutputID!="" ||
		st.OutputError!="OUTPUT_ROUTE_PARTIAL_RECONCILIATION_REQUIRED"{
		t.Fatalf("partially moved decks falsely reported success: %+v %v",st,err)
	}
	if len(f.calls)!=2{t.Fatalf("expected A then B attempted: %+v",f.calls)}
}
func TestAudioDeviceListInvalidJSONFailsClosed(t *testing.T){
	c,f:=routeFixture(t)
	f.queryResult=json.RawMessage(`{"unexpected":"object"}`)
	_,err:=c.Outputs(context.Background())
	if err==nil{t.Fatal("malformed response accepted")}
	f.queryResult=json.RawMessage(`[{"name":"auto"},{"name":"pulse/bluez_output.xx"}]`)
	list,err:=c.Outputs(context.Background())
	if err!=nil||len(list)!=1||!strings.Contains(list[0].ID,"bluez"){t.Fatalf("unexpected list %+v %v",list,err)}
}
