package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

func startOperatorTLS(handler http.Handler, certificate tls.Certificate, listenAddress string, errCh chan<- error) (*http.Server, error) {
	if handler == nil {
		return nil, fmt.Errorf("Operator HTTPS handler is required")
	}
	if _, _, err := net.SplitHostPort(strings.TrimSpace(listenAddress)); err != nil {
		return nil, fmt.Errorf("invalid Operator HTTPS listen address %q: %w", listenAddress, err)
	}
	server := &http.Server{
		Addr:              strings.TrimSpace(listenAddress),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{certificate},
		},
	}
	go func() {
		errCh <- server.ListenAndServeTLS("", "")
	}()
	return server, nil
}

func redirectRemoteOperatorHTTP(next http.Handler, secureHost, secureListen string) http.Handler {
	_, port, err := net.SplitHostPort(strings.TrimSpace(secureListen))
	if err != nil || strings.TrimSpace(secureHost) == "" {
		return next
	}
	secureAuthority := net.JoinHostPort(strings.TrimSpace(secureHost), port)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if remoteIsLoopback(r.RemoteAddr) || strings.HasPrefix(r.URL.Path, "/health/") {
			next.ServeHTTP(w, r)
			return
		}
		target := "https://" + secureAuthority + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}

func remoteIsLoopback(remote string) bool {
	remote = strings.TrimSpace(remote)
	if address, err := netip.ParseAddrPort(remote); err == nil {
		return address.Addr().IsLoopback()
	}
	address, err := netip.ParseAddr(remote)
	return err == nil && address.IsLoopback()
}
