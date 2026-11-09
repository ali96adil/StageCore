package camerarelay

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
)

type AccessScope uint8

const (
	ViewCamera AccessScope = iota
	ControlFlash
)

// FileAccess reloads credentials at each check so atomic rotation also revokes
// active viewers. Callers must serve the credentials only over a trusted channel.
func FileAccess(viewerFile, controlFile string) (func(*http.Request, AccessScope) bool, error) {
	if _, _, err := readAccessPair(viewerFile, controlFile); err != nil {
		return nil, err
	}
	return func(req *http.Request, scope AccessScope) bool {
		headers := req.Header.Values("Authorization")
		if len(headers) != 1 {
			return false
		}
		scheme, token, ok := strings.Cut(headers[0], " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || !validAccessToken(token) {
			return false
		}
		view, control, err := readAccessPair(viewerFile, controlFile)
		if err != nil {
			return false
		}
		digest := sha256.Sum256([]byte(token))
		controlMatch := subtle.ConstantTimeCompare(digest[:], control[:])
		if scope == ControlFlash {
			return controlMatch == 1
		}
		return controlMatch|subtle.ConstantTimeCompare(digest[:], view[:]) == 1
	}, nil
}

func readAccessPair(viewerFile, controlFile string) ([32]byte, [32]byte, error) {
	view, err := readAccessToken(viewerFile)
	if err != nil {
		return view, [32]byte{}, err
	}
	control, err := readAccessToken(controlFile)
	if err == nil && view == control {
		err = errors.New("viewer and control tokens must be different")
	}
	return view, control, err
}

func readAccessToken(path string) ([32]byte, error) {
	token, err := ReadViewerToken(path)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256([]byte(token)), nil
}

// ReadViewerToken loads an owner-only credential for a trusted local relay client.
func ReadViewerToken(path string) (string, error) {
	expected, err := os.Lstat(path)
	if err != nil || !expected.Mode().IsRegular() || expected.Mode().Perm()&0077 != 0 {
		return "", errors.New("camera tokens require owner-only regular files")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("camera token file is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !os.SameFile(expected, info) || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("camera tokens require owner-only regular files")
	}
	raw, err := io.ReadAll(io.LimitReader(file, 260))
	token := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if err != nil || !validAccessToken(token) {
		return "", errors.New("invalid camera token file")
	}
	return token, nil
}

func validAccessToken(token string) bool {
	if len(token) < 32 || len(token) > 256 {
		return false
	}
	for _, ch := range token {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' ||
			ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

func (r *Relay) requireAccess(w http.ResponseWriter, req *http.Request, scope AccessScope) bool {
	if r.cfg.Authorize == nil || r.cfg.Authorize(req, scope) {
		return true
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="stagecore-camera-relay"`)
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "camera access denied", http.StatusUnauthorized)
	return false
}
