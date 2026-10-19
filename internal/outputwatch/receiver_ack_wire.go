package outputwatch

import (
 "bytes"
 "crypto/hmac"
 "crypto/sha256"
 "encoding/binary"
 "encoding/hex"
 "strconv"
 "strings"
 "unicode/utf8"

 "github.com/ali96adil/StageCore/internal/osc"
)

// ReceiverAckAddress is reserved for a future authenticated application
// adapter. No Hub/Companion/UDP listener registers this address.
const ReceiverAckAddress = "/stagecore/receiver/ack/v1"

const (
 receiverAckVersion = "1"
 receiverAckFields = 13
 receiverAckMaximumPacket = 4096
 receiverAckMinKeyBytes = 32
)

// receiverWireFields uses OSC strings exclusively: generation/sequence are
// decimal strings to preserve uint64 precision in Max/VDMX/JS. All fields
// are authenticated, including receiver identity and receiving-app claim.
func receiverWireFields(n ReceiverNotice) ([]string,error) {
 s:=n.Scope
 if !s.valid() || strings.TrimSpace(n.ExecutionID)=="" ||
  strings.TrimSpace(n.CueID)=="" || len(n.Challenge)<16 || n.Sequence==0 ||
  (n.Status!=ReceiptAppReceived && n.Status!=ReceiptAppApplied) {
  return nil,ErrReceiptUntrusted
 }
 fields:=[]string{
  receiverAckVersion,
  s.ProjectID,s.RuntimeSnapshotID,s.MachineRoleID,s.CompanionID,
  s.ReceiverInstanceID,string(s.Kind),
  strconv.FormatInt(s.ConnectionGeneration,10),
  n.ExecutionID,n.CueID,n.Challenge,
  strconv.FormatUint(n.Sequence,10),string(n.Status),
 }
 for _,f:=range fields {
  if f==""||len(f)>128||!utf8.ValidString(f)||strings.ContainsAny(f,"\x00\r\n") {
   return nil,ErrReceiptUntrusted
  }
 }
 return fields,nil
}

// receiverMAC creates an unambiguous, domain-separated canonical signature
// over all fields using fixed 4-byte length-prefix encoding. The MAC is
// not an unscoped application token and never authorizes Cue execution.
func receiverMAC(key []byte, fields []string) ([]byte,error) {
 if len(key)<receiverAckMinKeyBytes||len(fields)!=receiverAckFields{
  return nil,ErrReceiptUntrusted
 }
 mac:=hmac.New(sha256.New,key)
 _,_=mac.Write([]byte("stagecore/receiver-ack/v1\x00"))
 var n [4]byte
 for _,field:=range fields {
  binary.BigEndian.PutUint32(n[:],uint32(len(field)))
  _,_=mac.Write(n[:])
  _,_=mac.Write([]byte(field))
 }
 return mac.Sum(nil),nil
}

// EncodeSignedReceiverAck is for test fixtures and a future explicitly
// provisioned receiver-side adapter. The key must be unique per receiver,
// at least 32 random bytes and NEVER transmitted in OSC packets/logs.
func EncodeSignedReceiverAck(n ReceiverNotice,key []byte)([]byte,error){
 fields,err:=receiverWireFields(n)
 if err!=nil{return nil,err}
 tag,err:=receiverMAC(key,fields)
 if err!=nil{return nil,err}
 args:=make([]osc.Argument,0,receiverAckFields+1)
 for _,field:=range fields {args=append(args,osc.Argument{Type:"string",Value:field})}
 args=append(args,osc.Argument{Type:"string",Value:hex.EncodeToString(tag)})
 packet,err:=osc.EncodeMessage(osc.Message{Address:ReceiverAckAddress,Arguments:args})
 if err!=nil||len(packet)>receiverAckMaximumPacket{return nil,ErrReceiptUntrusted}
 return packet,nil
}

// DecodeVerifiedReceiverAck verifies a per-receiver HMAC in constant time
// BEFORE exposing the claimed status. Caller supplies a trusted per-receiver
// key, never one selected solely from unauthenticated OSC input.
func DecodeVerifiedReceiverAck(packet,key []byte)(ReceiverNotice,error){
 if len(packet)==0||len(packet)>receiverAckMaximumPacket||
  len(key)<receiverAckMinKeyBytes{return ReceiverNotice{},ErrReceiptUntrusted}
 message,err:=osc.DecodeMessage(packet)
 if err!=nil||message.Address!=ReceiverAckAddress||
  len(message.Arguments)!=receiverAckFields+1{return ReceiverNotice{},ErrReceiptUntrusted}
 // Reject trailing bytes, altered padding and noncanonical OSC encodings.
 canonical,err:=osc.EncodeMessage(message)
 if err!=nil||!bytes.Equal(canonical,packet){return ReceiverNotice{},ErrReceiptUntrusted}
 fields:=make([]string,0,receiverAckFields)
 for i,a:=range message.Arguments{
  if a.Type!="string"{return ReceiverNotice{},ErrReceiptUntrusted}
  v,ok:=a.Value.(string)
  if !ok{return ReceiverNotice{},ErrReceiptUntrusted}
  if i<receiverAckFields {fields=append(fields,v)}
 }
 if fields[0]!=receiverAckVersion{return ReceiverNotice{},ErrReceiptUntrusted}
 macText:=message.Arguments[receiverAckFields].Value.(string)
 if len(macText)!=sha256.Size*2 || macText!=strings.ToLower(macText){
  return ReceiverNotice{},ErrReceiptUntrusted
 }
 supplied,err:=hex.DecodeString(macText)
 if err!=nil{return ReceiverNotice{},ErrReceiptUntrusted}
 expected,err:=receiverMAC(key,fields)
 if err!=nil||!hmac.Equal(supplied,expected){return ReceiverNotice{},ErrReceiptUntrusted}
 generation,err:=strconv.ParseInt(fields[7],10,64)
 if err!=nil||generation<=0||strconv.FormatInt(generation,10)!=fields[7]{
  return ReceiverNotice{},ErrReceiptUntrusted
 }
 seq,err:=strconv.ParseUint(fields[11],10,64)
 if err!=nil||seq==0||strconv.FormatUint(seq,10)!=fields[11]{
  return ReceiverNotice{},ErrReceiptUntrusted
 }
 n:=ReceiverNotice{
  Scope:ReceiverScope{ProjectID:fields[1],RuntimeSnapshotID:fields[2],
   MachineRoleID:fields[3],CompanionID:fields[4],
   ReceiverInstanceID:fields[5],Kind:ReceiverKind(fields[6]),
   ConnectionGeneration:generation},
  ExecutionID:fields[8],CueID:fields[9],Challenge:fields[10],
  Sequence:seq,Status:ReceiverReceiptState(fields[12]),
 }
 if _,err:=receiverWireFields(n);err!=nil{return ReceiverNotice{},err}
 return n,nil
}

// ObserveSignedReceiverAck is the only proposed bridge into the existing
// read-only receipt state machine. Decoding/authentication must succeed
// first; TRACKING itself remains unwired and no device action is possible.
func ObserveSignedReceiverAck(t *ReceiverReceiptTracker,packet,key []byte)error{
 if t==nil{return ErrReceiptUntrusted}
 n,err:=DecodeVerifiedReceiverAck(packet,key)
 if err!=nil{return ErrReceiptUntrusted}
 return t.Observe(n)
}
