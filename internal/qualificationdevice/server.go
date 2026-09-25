package qualificationdevice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/devicechannel"
)

const MaxRequestBytes = 16 << 10

var allowedCommands = map[string]bool{
	"TABLET_PREPARE":         true,
	"LIGHTING_CHANNELS_SET":  true,
	"LIGHTING_CHANNELS_FADE": true,
	"LIGHTING_STATE_READ":    true,
	"LIGHTING_CONFIG_READ":   true,
}

type exchangeRuntime interface {
	QualificationExchange(context.Context, string, contracts.CommandEnvelope) (devicechannel.QualificationExchangeResult, error)
}

type Server struct {
	path   string
	server *http.Server
	listen net.Listener
	errs   chan error
	once   sync.Once
}

type requestBody struct {
	DeviceID string                    `json:"device_id"`
	Command  contracts.CommandEnvelope `json:"command"`
}

func Start(ctx context.Context, runtime exchangeRuntime, socketPath string) (*Server, error) {
	if runtime == nil {
		return nil, errors.New("qualification runtime is required")
	}
	socketPath = strings.TrimSpace(socketPath)
	if !filepath.IsAbs(socketPath) {
		return nil, errors.New("qualification socket path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o750); err != nil {
		return nil, fmt.Errorf("create qualification socket directory: %w", err)
	}
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("qualification socket path exists and is not a socket")
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("remove stale qualification socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen qualification socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("secure qualification socket: %w", err)
	}

	s := &Server{path: socketPath, listen: listener, errs: make(chan error, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/device-envelope", s.handle(runtime))
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	go func() {
		err := s.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		s.errs <- err
		close(s.errs)
	}()
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	return s, nil
}

func (s *Server) Errors() <-chan error {
	if s == nil {
		return nil
	}
	return s.errs
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var result error
	s.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if s.server != nil {
			result = s.server.Shutdown(ctx)
		}
		if s.listen != nil {
			_ = s.listen.Close()
		}
		if s.path != "" {
			_ = os.Remove(s.path)
		}
	})
	return result
}

func (s *Server) handle(runtime exchangeRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
		defer r.Body.Close()
		var input requestBody
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "invalid qualification envelope request", http.StatusBadRequest)
			return
		}
		input.DeviceID = strings.TrimSpace(input.DeviceID)
		input.Command.CommandID = strings.TrimSpace(input.Command.CommandID)
		input.Command.CommandType = strings.TrimSpace(input.Command.CommandType)
		input.Command.ProjectID = strings.TrimSpace(input.Command.ProjectID)
		input.Command.Issuer = strings.TrimSpace(input.Command.Issuer)
		if input.DeviceID == "" ||
			!strings.HasPrefix(input.Command.CommandID, "qualification-") ||
			!allowedCommands[input.Command.CommandType] ||
			input.Command.SchemaVersion != contracts.SchemaVersion1 ||
			input.Command.ProjectID == "" ||
			input.Command.Issuer != "qualification:physical-runner" {
			http.Error(w, "qualification envelope rejected", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 140*time.Second)
		defer cancel()
		result, err := runtime.QualificationExchange(ctx, input.DeviceID, input.Command)
		if err != nil {
			http.Error(w, "qualification exchange unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}
