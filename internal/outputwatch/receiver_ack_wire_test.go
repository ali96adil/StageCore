package outputwatch

import (
 "bytes"
 "crypto/rand"
 "errors"
 "strings"
 "testing"
 "time"

 "github.com/ali96adil/StageCore/internal/osc"
)
func testAckKey(t *testing.T)[]byte{
 t.Helper()
 k:=make([]byte,32)
 if _,err:=rand.Read(k);err!=nil{t.Fatal(err)}
 return k
}
func TestSignedReceiverOSCAckRoundTripHMACAndApplicationClaims(t *testing.T){
 tracker,_,scope,dispatch:=receiverFixture()
 key:=testAckKey(t)
 if err:=tracker.BindQualified(scope);err!=nil{t.Fatal(err)}
 if err:=tracker.TrackTransportSent(dispatch);err!=nil{t.Fatal(err)}
 recv:=ReceiverNotice{Scope:scope,ExecutionID:dispatch.ExecutionID,CueID:dispatch.CueID,
  Challenge:dispatch.Challenge,Sequence:1,Status:ReceiptAppReceived}
 packet,err:=EncodeSignedReceiverAck(recv,key)
 if err!=nil{t.Fatal(err)}
 if !bytes.Contains(packet,[]byte(ReceiverAckAddress)){t.Fatal("wrong OSC address")}
 decoded,err:=DecodeVerifiedReceiverAck(packet,key)
 if err!=nil||decoded!=recv{t.Fatalf("OSC signature roundtrip: %+v %v",decoded,err)}
 if err:=ObserveSignedReceiverAck(tracker,packet,key);err!=nil{t.Fatalf("receiver RECEIVED rejected: %v",err)}
 v,ok:=tracker.View(dispatch.ExecutionID)
 if !ok||v.State!=ReceiptAppReceived||v.PhysicalOutputVerified||v.CommandsEnabled{
  t.Fatalf("signed RECEIVED claimed verified physical state: %+v",v)
 }
 applied:=recv;applied.Sequence=2;applied.Status=ReceiptAppApplied
 finalPacket,err:=EncodeSignedReceiverAck(applied,key)
 if err!=nil{t.Fatal(err)}
 if err:=ObserveSignedReceiverAck(tracker,finalPacket,key);err!=nil{t.Fatal(err)}
 v,_=tracker.View(dispatch.ExecutionID)
 if v.State!=ReceiptAppApplied||v.PhysicalOutputVerified||v.CommandsEnabled{
  t.Fatalf("software ACK used as actuation proof: %+v",v)
 }
 if err:=ObserveSignedReceiverAck(tracker,finalPacket,key);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("signed ACK replay accepted")
 }
}
func TestSignedReceiverOSCAckRejectsTamperingWrongKeyAndNoncanonicalPackets(t *testing.T){
 r,_,scope,d:=receiverFixture()
 key:=testAckKey(t)
 if err:=r.BindQualified(scope);err!=nil{t.Fatal(err)}
 if err:=r.TrackTransportSent(d);err!=nil{t.Fatal(err)}
 n:=ReceiverNotice{Scope:scope,ExecutionID:d.ExecutionID,CueID:d.CueID,
  Challenge:d.Challenge,Sequence:1,Status:ReceiptAppReceived}
 good,err:=EncodeSignedReceiverAck(n,key)
 if err!=nil{t.Fatal(err)}
 wrong:=testAckKey(t)
 if _,err:=DecodeVerifiedReceiverAck(good,wrong);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("wrong receiver key accepted")
 }
 packet:=append([]byte(nil),good...)
 index:=bytes.Index(packet,[]byte(n.CueID))
 if index<0{t.Fatal("CueID not found in OSC fixture")}
 packet[index]='X'
 if _,err:=DecodeVerifiedReceiverAck(packet,key);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("modified CueID accepted with old HMAC")
 }
 forged,err:=osc.DecodeMessage(good)
 if err!=nil{t.Fatal(err)}
 forged.Arguments[12]=osc.Argument{Type:"string",Value:string(ReceiptAppApplied)}
 unsignedEdit,err:=osc.EncodeMessage(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=DecodeVerifiedReceiverAck(unsignedEdit,key);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("changed APPLIED status accepted")
 }
 wrongAddress,err:=osc.EncodeMessage(osc.Message{Address:"/stagecore/go",Arguments:forged.Arguments})
 if err!=nil{t.Fatal(err)}
 if _,err:=DecodeVerifiedReceiverAck(wrongAddress,key);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("unrelated OSC address accepted")
 }
 trailing:=append(append([]byte(nil),good...),0,0,0,0)
 if _,err:=DecodeVerifiedReceiverAck(trailing,key);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("trailing data accepted")
 }
 missing:=good[:len(good)-1]
 if _,err:=DecodeVerifiedReceiverAck(missing,key);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("truncated HMAC accepted")
 }
 tooLarge:=bytes.Repeat([]byte{0},receiverAckMaximumPacket+1)
 if _,err:=DecodeVerifiedReceiverAck(tooLarge,key);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("oversized UDP packet accepted")
 }
 if err:=ObserveSignedReceiverAck(r,packet,key);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("tampered packet updated state")
 }
 v,_:=r.View(d.ExecutionID)
 if v.State!=ReceiptTransportOnly{t.Fatalf("invalid signature advanced application state: %+v",v)}
}
func TestSignedReceiverAckRejectsBadFieldsKeyAndExpiredReceipt(t *testing.T){
 r,clock,scope,d:=receiverFixture()
 key:=testAckKey(t)
 n:=ReceiverNotice{Scope:scope,ExecutionID:d.ExecutionID,CueID:d.CueID,
  Challenge:d.Challenge,Sequence:1,Status:ReceiptAppReceived}
 short:=[]byte("insecure")
 if _,err:=EncodeSignedReceiverAck(n,short);!errors.Is(err,ErrReceiptUntrusted){t.Fatal("short key accepted")}
 invalids:=[]ReceiverNotice{}
 bad:=n;bad.Scope.ConnectionGeneration=0;invalids=append(invalids,bad)
 bad=n;bad.Sequence=0;invalids=append(invalids,bad)
 bad=n;bad.CueID=strings.Repeat("x",129);invalids=append(invalids,bad)
 bad=n;bad.Challenge="short";invalids=append(invalids,bad)
 bad=n;bad.Status=ReceiptTransportOnly;invalids=append(invalids,bad)
 bad=n;bad.ExecutionID="evil\x00id";invalids=append(invalids,bad)
 for i,x:=range invalids{
  if _,err:=EncodeSignedReceiverAck(x,key);!errors.Is(err,ErrReceiptUntrusted){
   t.Fatalf("invalid source fields %d signed",i)
  }
 }
 if err:=r.BindQualified(scope);err!=nil{t.Fatal(err)}
 if err:=r.TrackTransportSent(d);err!=nil{t.Fatal(err)}
 packet,err:=EncodeSignedReceiverAck(n,key)
 if err!=nil{t.Fatal(err)}
 *clock=clock.Add(6*time.Second)
 if err:=ObserveSignedReceiverAck(r,packet,key);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("cryptographically valid but expired receipt accepted")
 }
 view,_:=r.View(d.ExecutionID)
 if view.State!=ReceiptUnconfirmed{t.Fatalf("expired receipt upgraded state: %+v",view)}
}
func TestSignedReceiverAckNeverDispatchesToHardware(t *testing.T){
 r,_,scope,d:=receiverFixture()
 key:=testAckKey(t)
 if err:=r.BindQualified(scope);err!=nil{t.Fatal(err)}
 if err:=r.TrackTransportSent(d);err!=nil{t.Fatal(err)}
 msg:=ReceiverNotice{Scope:scope,ExecutionID:d.ExecutionID,CueID:d.CueID,Challenge:d.Challenge,
  Sequence:1,Status:ReceiptAppReceived}
 packet,err:=EncodeSignedReceiverAck(msg,key)
 if err!=nil{t.Fatal(err)}
 if err:=ObserveSignedReceiverAck(r,packet,key);err!=nil{t.Fatal(err)}
 view,_:=r.View(d.ExecutionID)
 if view.CommandsEnabled||view.PhysicalOutputVerified{
  t.Fatalf("receiver application acknowledgment enabled commands: %+v",view)
 }
 // No dispatch entrypoint, socket binding, hardware action or runtime
 // Session state mutation exists in this codec.
 _=time.Second
}
