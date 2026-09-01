package extension

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	RuntimeNetworkBrokerSchemaVersion      = 1
	RuntimeNetworkBrokerPermissionUDPSend  = "network.udp.send"
	RuntimeNetworkBrokerOperationUDPSend   = "udp.send"
	RuntimeNetworkBrokerRequestType        = "network.request"
	RuntimeNetworkBrokerResultType         = "network.result"
	RuntimeNetworkBrokerSocketEnv          = "STAGECORE_NETWORK_BROKER"
	RuntimeNetworkBrokerTokenEnv           = "STAGECORE_NETWORK_BROKER_TOKEN"
	RuntimeNetworkBrokerSandboxDirectory   = "/stagecore/network"
	RuntimeNetworkBrokerSandboxSocket      = "/stagecore/network/n.sock"
	runtimeNetworkBrokerSocketName         = "n.sock"
	runtimeNetworkBrokerMaxRequestBytes    = 128 * 1024
	runtimeNetworkBrokerMaxUDPPayloadBytes = 65507
	runtimeNetworkBrokerMaxConnections     = 4
)

const (
	RuntimeNetworkBrokerStatusCompleted = "COMPLETED"
	RuntimeNetworkBrokerStatusFailed    = "FAILED"
)

const (
	RuntimeNetworkBrokerErrorAuth       = "BROKER_AUTH_FAILED"
	RuntimeNetworkBrokerErrorPermission = "BROKER_PERMISSION_DENIED"
	RuntimeNetworkBrokerErrorRequest    = "BROKER_REQUEST_INVALID"
	RuntimeNetworkBrokerErrorTarget     = "BROKER_TARGET_INVALID"
	RuntimeNetworkBrokerErrorPayload    = "BROKER_PAYLOAD_INVALID"
	RuntimeNetworkBrokerErrorUDPSend    = "BROKER_UDP_SEND_FAILED"
)

type RuntimeNetworkBrokerRequest struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	RequestID     string `json:"request_id"`
	Operation     string `json:"operation"`
	Token         string `json:"token"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	PayloadBase64 string `json:"payload_base64"`
}

type RuntimeNetworkBrokerResponse struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	RequestID     string `json:"request_id,omitempty"`
	Status        string `json:"status"`
	BytesSent     int    `json:"bytes_sent,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
}

type RuntimeNetworkBroker struct {
	root string
}

type RuntimeNetworkBrokerSession struct {
	broker      *RuntimeNetworkBroker
	directory   string
	socketPath  string
	token       string
	permissions map[string]struct{}
	listener    *net.UnixListener
	semaphore   chan struct{}
	done        chan struct{}

	closeOnce sync.Once
	wg        sync.WaitGroup
	mu        sync.Mutex
	conns     map[*net.UnixConn]struct{}
}

func NewRuntimeNetworkBroker(installer *Installer) (*RuntimeNetworkBroker, error) {
	if installer == nil || strings.TrimSpace(installer.root) == "" {
		return nil, fmt.Errorf("runtime network broker requires installer")
	}
	runtimeRoot := filepath.Join(installer.root, "runtime")
	root := filepath.Join(runtimeRoot, "broker")
	if err := ensureManagedDirectory(runtimeRoot); err != nil {
		return nil, err
	}
	if err := ensureManagedDirectory(root); err != nil {
		return nil, err
	}
	broker := &RuntimeNetworkBroker{root: root}
	if err := broker.cleanRoot(); err != nil {
		return nil, err
	}
	return broker, nil
}

func (b *RuntimeNetworkBroker) Supports(permissions []string) bool {
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if !strings.HasPrefix(permission, "network.") {
			continue
		}
		if permission != RuntimeNetworkBrokerPermissionUDPSend {
			return false
		}
	}
	return true
}

func (b *RuntimeNetworkBroker) OpenSession(permissions []string) (*RuntimeNetworkBrokerSession, error) {
	if b == nil || strings.TrimSpace(b.root) == "" {
		return nil, fmt.Errorf("runtime network broker is unavailable")
	}
	if !b.Supports(permissions) {
		return nil, fmt.Errorf("runtime network broker does not support requested network permissions")
	}
	if err := inspectManagedDirectory(b.root); err != nil {
		return nil, err
	}

	directory, err := os.MkdirTemp(b.root, "b-")
	if err != nil {
		return nil, fmt.Errorf("create runtime network broker directory: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(directory)
		}
	}()
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("secure runtime network broker directory: %w", err)
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate runtime network broker token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	socketPath := filepath.Join(directory, runtimeNetworkBrokerSocketName)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("listen on runtime network broker socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("secure runtime network broker socket: %w", err)
	}

	permissionSet := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if permission != "" {
			permissionSet[permission] = struct{}{}
		}
	}
	session := &RuntimeNetworkBrokerSession{
		broker:      b,
		directory:   directory,
		socketPath:  socketPath,
		token:       token,
		permissions: permissionSet,
		listener:    listener,
		semaphore:   make(chan struct{}, runtimeNetworkBrokerMaxConnections),
		done:        make(chan struct{}),
		conns:       make(map[*net.UnixConn]struct{}),
	}
	keep = true
	session.wg.Add(1)
	go session.serve()
	return session, nil
}

func (s *RuntimeNetworkBrokerSession) HostDirectory() string {
	if s == nil {
		return ""
	}
	return s.directory
}

func (s *RuntimeNetworkBrokerSession) HostSocketPath() string {
	if s == nil {
		return ""
	}
	return s.socketPath
}

func (s *RuntimeNetworkBrokerSession) Token() string {
	if s == nil {
		return ""
	}
	return s.token
}

func (s *RuntimeNetworkBrokerSession) Close() error {
	if s == nil {
		return nil
	}
	var closeErr error
	s.closeOnce.Do(func() {
		close(s.done)
		if s.listener != nil {
			if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				closeErr = errors.Join(closeErr, err)
			}
		}
		s.mu.Lock()
		for conn := range s.conns {
			_ = conn.Close()
		}
		s.mu.Unlock()
		s.wg.Wait()
		if err := os.RemoveAll(s.directory); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
		if s.broker != nil {
			if err := syncActivationDirectory(s.broker.root); err != nil {
				closeErr = errors.Join(closeErr, err)
			}
		}
	})
	return closeErr
}

func (s *RuntimeNetworkBrokerSession) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		select {
		case s.semaphore <- struct{}{}:
		default:
			_ = conn.Close()
			continue
		}
		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *RuntimeNetworkBrokerSession) handleConnection(conn *net.UnixConn) {
	defer s.wg.Done()
	defer func() { <-s.semaphore }()
	defer conn.Close()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4*1024), runtimeNetworkBrokerMaxRequestBytes)
	for scanner.Scan() {
		response := s.handleRequest(scanner.Bytes())
		if err := json.NewEncoder(conn).Encode(response); err != nil {
			return
		}
	}
}

func (s *RuntimeNetworkBrokerSession) handleRequest(raw []byte) RuntimeNetworkBrokerResponse {
	response := RuntimeNetworkBrokerResponse{
		Type:          RuntimeNetworkBrokerResultType,
		SchemaVersion: RuntimeNetworkBrokerSchemaVersion,
		Status:        RuntimeNetworkBrokerStatusFailed,
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request RuntimeNetworkBrokerRequest
	if err := decoder.Decode(&request); err != nil {
		response.ErrorCode = RuntimeNetworkBrokerErrorRequest
		response.ErrorMessage = "invalid network broker request"
		return response
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		response.ErrorCode = RuntimeNetworkBrokerErrorRequest
		response.ErrorMessage = "network broker request contains trailing data"
		return response
	}
	response.RequestID = strings.TrimSpace(request.RequestID)
	if request.Type != RuntimeNetworkBrokerRequestType || request.SchemaVersion != RuntimeNetworkBrokerSchemaVersion || response.RequestID == "" {
		response.ErrorCode = RuntimeNetworkBrokerErrorRequest
		response.ErrorMessage = "network broker request contract mismatch"
		return response
	}
	if subtle.ConstantTimeCompare([]byte(request.Token), []byte(s.token)) != 1 {
		response.ErrorCode = RuntimeNetworkBrokerErrorAuth
		response.ErrorMessage = "network broker authentication failed"
		return response
	}
	if request.Operation != RuntimeNetworkBrokerOperationUDPSend {
		response.ErrorCode = RuntimeNetworkBrokerErrorPermission
		response.ErrorMessage = "network broker operation is not authorized"
		return response
	}
	if _, ok := s.permissions[RuntimeNetworkBrokerPermissionUDPSend]; !ok {
		response.ErrorCode = RuntimeNetworkBrokerErrorPermission
		response.ErrorMessage = "network.udp.send permission is not approved"
		return response
	}

	host := strings.TrimSpace(request.Host)
	ip := net.ParseIP(host)
	if ip == nil || request.Port < 1 || request.Port > 65535 || ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
		response.ErrorCode = RuntimeNetworkBrokerErrorTarget
		response.ErrorMessage = "UDP target must be an explicit unicast IP and valid port"
		return response
	}
	payload, err := base64.StdEncoding.DecodeString(request.PayloadBase64)
	if err != nil || len(payload) > runtimeNetworkBrokerMaxUDPPayloadBytes {
		response.ErrorCode = RuntimeNetworkBrokerErrorPayload
		response.ErrorMessage = "UDP payload is invalid or too large"
		return response
	}

	udp, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: ip, Port: request.Port})
	if err != nil {
		response.ErrorCode = RuntimeNetworkBrokerErrorUDPSend
		response.ErrorMessage = "UDP target is unavailable"
		return response
	}
	defer udp.Close()
	_ = udp.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	written, err := udp.Write(payload)
	if err != nil || written != len(payload) {
		response.ErrorCode = RuntimeNetworkBrokerErrorUDPSend
		response.ErrorMessage = "UDP send failed"
		return response
	}
	response.Status = RuntimeNetworkBrokerStatusCompleted
	response.BytesSent = written
	return response
}

func (b *RuntimeNetworkBroker) cleanRoot() error {
	if err := inspectManagedDirectory(b.root); err != nil {
		return err
	}
	entries, err := os.ReadDir(b.root)
	if err != nil {
		return fmt.Errorf("read runtime network broker root: %w", err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "b-") {
			return fmt.Errorf("unexpected entry in managed runtime network broker root: %s", entry.Name())
		}
		fullPath := filepath.Join(b.root, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected runtime network broker entry: %s", entry.Name())
		}
		if err := os.RemoveAll(fullPath); err != nil {
			return fmt.Errorf("remove stale runtime network broker session: %w", err)
		}
	}
	return syncActivationDirectory(b.root)
}

func runtimeNetworkPermissions(permissions []string) []string {
	out := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if strings.HasPrefix(permission, "network.") {
			out = append(out, permission)
		}
	}
	return out
}

func runtimeNetworkBrokerContext() context.Context {
	return context.Background()
}
