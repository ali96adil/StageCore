package osc

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestSenderSendsRealUDPOSCMessage(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	port := conn.LocalAddr().(*net.UDPAddr).Port
	received := make(chan Message, 1)
	errors := make(chan error, 1)
	go func() {
		buf := make([]byte, 2048)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			errors <- err
			return
		}
		msg, err := DecodeMessage(buf[:n])
		if err != nil {
			errors <- err
			return
		}
		received <- msg
	}()

	sender := Sender{WriteTimeout: time.Second}
	result, err := sender.Send(context.Background(), Endpoint{ID: "video-main", Host: "127.0.0.1", Port: port}, Message{
		Address:   "/scene/go",
		Arguments: []Argument{{Type: "int32", Value: 4}, {Type: "string", Value: "Intro"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "COMPLETED" || result.AckLevel != "TRANSPORT_ONLY" || result.BytesSent == 0 {
		t.Fatalf("unexpected result: %#v", result)
	}

	select {
	case err := <-errors:
		t.Fatal(err)
	case msg := <-received:
		if msg.Address != "/scene/go" {
			t.Fatalf("address=%q", msg.Address)
		}
		if msg.Arguments[0].Value.(int32) != 4 {
			t.Fatalf("arg0=%v", msg.Arguments[0].Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("receiver did not get OSC packet")
	}
}

func TestSenderRejectsInvalidEndpoint(t *testing.T) {
	sender := Sender{}
	result, err := sender.Send(context.Background(), Endpoint{Host: "", Port: 9000}, Message{Address: "/go"})
	if err == nil || result.Status != "FAILED" || result.AckLevel != "NONE" {
		t.Fatalf("expected explicit failed result, got %#v err=%v", result, err)
	}
}
