package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: stagecore-pairing approve|revoke [options]")
	}
	switch os.Args[1] {
	case "approve":
		approve(os.Args[2:])
	case "revoke":
		revoke(os.Args[2:])
	default:
		fail("unknown pairing command")
	}
}

func approve(args []string) {
	fs := flag.NewFlagSet("approve", flag.ExitOnError)
	dataRoot := fs.String("data-root", defaultDataRoot(), "authoritative StageCore data root")
	requestID := fs.String("request-id", "", "pending pairing request id")
	actor := fs.String("actor", "", "authorized local operator identity")
	_ = fs.Parse(args)
	if strings.TrimSpace(*requestID) == "" || strings.TrimSpace(*actor) == "" {
		fail("request-id and actor are required")
	}
	fmt.Fprint(os.Stderr, "Pairing code: ")
	code, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fail("could not read pairing code")
	}
	service, closeDB := openService(*dataRoot)
	defer closeDB()
	key, err := service.ApprovePairing(context.Background(), *requestID, strings.TrimSpace(code), companionauth.Approval{
		Actor: *actor, Authorized: true,
	})
	if err != nil {
		fail("pairing approval failed: " + string(companionauth.ErrorCode(err)))
	}
	fmt.Printf("Companion %s paired with fingerprint %s\n", key.CompanionID, key.PublicKeyFingerprint)
}

func revoke(args []string) {
	fs := flag.NewFlagSet("revoke", flag.ExitOnError)
	dataRoot := fs.String("data-root", defaultDataRoot(), "authoritative StageCore data root")
	companionID := fs.String("companion-id", "", "trusted Companion id")
	actor := fs.String("actor", "", "authorized local operator identity")
	reason := fs.String("reason", "operator revocation", "non-secret audit reason")
	_ = fs.Parse(args)
	if strings.TrimSpace(*companionID) == "" || strings.TrimSpace(*actor) == "" {
		fail("companion-id and actor are required")
	}
	service, closeDB := openService(*dataRoot)
	defer closeDB()
	if err := service.Revoke(context.Background(), *companionID, *actor, *reason, true); err != nil {
		fail("Companion revocation failed")
	}
	fmt.Printf("Companion %s revoked\n", *companionID)
}

func openService(dataRoot string) (*companionauth.Service, func()) {
	handle, err := db.Open(context.Background(), db.Config{DataRoot: strings.TrimSpace(dataRoot)})
	if err != nil {
		fail("could not open StageCore data")
	}
	s := store.New(handle.DB, clock.Real{})
	return companionauth.New(s, nil), func() { _ = handle.Close() }
}

func defaultDataRoot() string {
	if value := strings.TrimSpace(os.Getenv("STAGECORE_DATA_ROOT")); value != "" {
		return value
	}
	return filepath.Join(".", "stagecore-data")
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
