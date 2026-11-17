package companionchannel

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/outputwatch"
)

// SignedACKConfig opts in one explicitly assigned receiving application.
type SignedACKConfig struct {
	ProjectID          string                   `json:"project_id"`
	MachineRoleID      string                   `json:"machine_role_id"`
	CompanionID        string                   `json:"companion_id"`
	ReceiverInstanceID string                   `json:"receiver_instance_id"`
	Kind               outputwatch.ReceiverKind `json:"kind"`
	Key                []byte                   `json:"key"`
}

func (c *RuntimeChannel) LoadSignedACKConfig(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("signed ACK configuration requires an absolute private file path")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("signed ACK configuration must be a private regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || opened.Mode().Perm()&0077 != 0 || opened.Size() > 4096 {
		return errors.New("signed ACK configuration file changed or is too large")
	}
	var cfg SignedACKConfig
	decoder := json.NewDecoder(io.LimitReader(file, 4097))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cfg) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid signed ACK configuration")
	}
	scope := cfg.scope("validation", 1)
	for _, id := range []string{scope.ProjectID, scope.MachineRoleID, scope.CompanionID, scope.ReceiverInstanceID} {
		if strings.TrimSpace(id) != id || len(id) == 0 || len(id) > 128 || strings.ContainsAny(id, "\x00\r\n") {
			return errors.New("invalid signed ACK identity")
		}
	}
	if _, err := outputwatch.NewSignedReceiver(scope, cfg.Key, nil); err != nil {
		return errors.New("invalid signed ACK scope or key")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.signedACK != nil || len(c.connections) != 0 {
		return errors.New("signed ACK configuration is startup-only")
	}
	c.signedACK = &cfg
	return nil
}

func (cfg SignedACKConfig) scope(snapshotID string, generation int64) outputwatch.ReceiverScope {
	return outputwatch.ReceiverScope{
		ProjectID: cfg.ProjectID, RuntimeSnapshotID: snapshotID, MachineRoleID: cfg.MachineRoleID,
		CompanionID: cfg.CompanionID, ReceiverInstanceID: cfg.ReceiverInstanceID, Kind: cfg.Kind,
		ConnectionGeneration: generation,
	}
}

func (cfg *SignedACKConfig) arm(req ExecutionRequest, generation int64) (*outputwatch.SignedReceiver, *outputwatch.ReceiverDispatch, error) {
	if cfg == nil || cfg.ProjectID != req.ProjectID || cfg.MachineRoleID != req.MachineRoleID ||
		cfg.CompanionID != req.CompanionID || (req.Capability != "midi.send" && req.Capability != "osc.send") {
		return nil, nil, errors.New("no matching signed ACK receiver")
	}
	receiver, err := outputwatch.NewSignedReceiver(cfg.scope(req.RuntimeSnapshotID, generation), cfg.Key, nil)
	if err != nil {
		return nil, nil, err
	}
	dispatch, err := receiver.Prepare(req.ExecutionID, req.CueID, time.Now().Add(30*time.Second))
	return receiver, &dispatch, err
}

func verifySignedACK(receiver *outputwatch.SignedReceiver, result ExecutionResult, packets [][]byte) ExecutionResult {
	if result.Result != domain.ExecutionCompleted {
		return result
	}
	if result.AckLevel != contracts.AckTransportOnly || len(packets) != 2 ||
		receiver.Accept(packets[0]) != nil || receiver.Accept(packets[1]) != nil {
		return failed(result.ExecutionID, "APPLICATION_ACK_INVALID", "Application confirmation failed after dispatch", domain.ExecutionFailed)
	}
	view, ok := receiver.View(result.ExecutionID)
	if !ok || view.State != outputwatch.ReceiptAppApplied {
		return failed(result.ExecutionID, "APPLICATION_ACK_INVALID", "Application did not confirm applying this execution", domain.ExecutionFailed)
	}
	result.AckLevel = contracts.AckApplication
	return result
}
