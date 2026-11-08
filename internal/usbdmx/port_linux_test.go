//go:build linux

package usbdmx

import (
	"context"
	"errors"
	"testing"
)

func TestLinuxSerialOpenRequiresIdentityApprovalBeforeTouchingDevices(t *testing.T) {
	// No real USB access in CI: verify fail-closed path and authorization
	// checks before Open() is allowed to be reached.
	ctx:=context.Background()
	for _,path:=range []string{
		"/dev/ttyUSB0", "/dev/serial/by-path/other",
		"/tmp/unverified-ttyUSB0", "/dev/serial/by-id/../../ttyUSB0",
	} {
		called:=false
		p,err:=OpenVerifiedLinuxPort(ctx,path,func(context.Context,string)error{
			called=true;return nil
		})
		if p!=nil || err==nil || called{t.Fatalf("invalid serial path %q accepted or authorized: %+v %v",path,p,err)}
	}
	p,err:=OpenVerifiedLinuxPort(ctx,"/dev/serial/by-id/no-such-device",nil)
	if p!=nil || !errors.Is(err,ErrAuthorizationRequired) {
		t.Fatalf("missing identity callback not rejected: %+v %v",p,err)
	}
}

func TestLinuxPortNilCannotWriteOrClaimSuccess(t *testing.T) {
	var p *LinuxPort
	if err:=p.WritePacket(context.Background(),ZeroFrame());!errors.Is(err,ErrQuarantined){
		t.Fatalf("nil Linux port wrote an output: %v",err)
	}
	if err:=p.Close();err!=nil{t.Fatal(err)}
}
