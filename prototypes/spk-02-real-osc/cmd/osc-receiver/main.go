package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-02-real-osc/internal/osc"
)

func main() {
	host := flag.String("host", "0.0.0.0", "listen host")
	port := flag.Int("port", 9000, "listen UDP port")
	timeout := flag.Duration("timeout", 30*time.Second, "maximum wait for one packet")
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", *host, *port))
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(*timeout)); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("listening for one OSC UDP packet on %s\n", conn.LocalAddr())

	buf := make([]byte, 64*1024)
	n, remote, err := conn.ReadFromUDP(buf)
	if err != nil {
		log.Fatal(err)
	}
	msg, err := osc.DecodeMessage(buf[:n])
	if err != nil {
		log.Fatalf("decode packet from %s: %v", remote, err)
	}
	out, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("from=%s bytes=%d\n%s\n", remote, n, out)
}
