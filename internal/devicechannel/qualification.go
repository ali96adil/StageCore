package devicechannel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/contracts"
)

var ErrQualificationExchangeUnavailable = errors.New("qualification Stage Device exchange unavailable")

type QualificationExchangeResult struct {
	CommandID string                  `json:"command_id"`
	Statuses  []contracts.CommandStatus `json:"statuses"`
	Status    contracts.CommandStatus `json:"status"`
	Payload   json.RawMessage         `json:"payload,omitempty"`
	Error     *contracts.ContractError `json:"error,omitempty"`
}

type qualificationWaiter struct {
	connection *connection
	results    chan inboundMessage
}

func (r *Runtime) QualificationExchange(
	ctx context.Context,
	deviceID string,
	envelope contracts.CommandEnvelope,
) (QualificationExchangeResult, error) {
	if r == nil {
		return QualificationExchangeResult{}, ErrQualificationExchangeUnavailable
	}
	deviceID = strings.TrimSpace(deviceID)
	envelope.CommandID = strings.TrimSpace(envelope.CommandID)
	envelope.CommandType = strings.TrimSpace(envelope.CommandType)
	envelope.ProjectID = strings.TrimSpace(envelope.ProjectID)
	envelope.Issuer = strings.TrimSpace(envelope.Issuer)
	if deviceID == "" || envelope.CommandID == "" || envelope.CommandType == "" ||
		envelope.ProjectID == "" || envelope.Issuer == "" ||
		envelope.SchemaVersion != contracts.SchemaVersion1 {
		return QualificationExchangeResult{}, fmt.Errorf("%w: incomplete qualification envelope", ErrQualificationExchangeUnavailable)
	}
	if len(envelope.CommandID) > 128 || len(envelope.CommandType) > 64 {
		return QualificationExchangeResult{}, fmt.Errorf("%w: qualification envelope exceeds bounds", ErrQualificationExchangeUnavailable)
	}

	waiter := &qualificationWaiter{results: make(chan inboundMessage, 8)}

	r.mu.Lock()
	current := r.connections[deviceID]
	if r.closed || current == nil {
		r.mu.Unlock()
		return QualificationExchangeResult{}, fmt.Errorf("%w: device is not connected", ErrQualificationExchangeUnavailable)
	}
	if r.inflight[envelope.CommandID] != nil || r.qualification[envelope.CommandID] != nil {
		r.mu.Unlock()
		return QualificationExchangeResult{}, fmt.Errorf("%w: command id is already active", ErrQualificationExchangeUnavailable)
	}
	waiter.connection = current
	r.qualification[envelope.CommandID] = waiter
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		if r.qualification[envelope.CommandID] == waiter {
			delete(r.qualification, envelope.CommandID)
		}
		r.mu.Unlock()
	}()

	if err := current.send(executeMessage{
		Type:          "command.execute",
		SchemaVersion: runtimeSchemaVersion,
		DeviceID:      deviceID,
		Command:       envelope,
	}); err != nil {
		return QualificationExchangeResult{}, fmt.Errorf("%w: %v", ErrQualificationExchangeUnavailable, err)
	}

	result := QualificationExchangeResult{CommandID: envelope.CommandID}
	for {
		select {
		case <-ctx.Done():
			return QualificationExchangeResult{}, ctx.Err()
		case <-current.closed:
			return QualificationExchangeResult{}, fmt.Errorf("%w: device connection ended", ErrQualificationExchangeUnavailable)
		case message := <-waiter.results:
			result.Statuses = append(result.Statuses, message.Status)
			if message.Status == contracts.CommandAccepted {
				continue
			}
			switch message.Status {
			case contracts.CommandRejected, contracts.CommandCompleted, contracts.CommandFailed, contracts.CommandTimedOut, contracts.CommandCancelled:
				result.Status = message.Status
				result.Payload = message.Payload
				result.Error = message.Error
				return result, nil
			default:
				return QualificationExchangeResult{}, fmt.Errorf("%w: invalid device result status %q", ErrQualificationExchangeUnavailable, message.Status)
			}
		}
	}
}

func (r *Runtime) deliverQualificationResult(message inboundMessage, current *connection) bool {
	if r == nil || current == nil {
		return false
	}
	commandID := strings.TrimSpace(message.CommandID)
	if commandID == "" {
		return false
	}
	r.mu.Lock()
	waiter := r.qualification[commandID]
	r.mu.Unlock()
	if waiter == nil || waiter.connection != current {
		return false
	}
	select {
	case waiter.results <- message:
		return true
	case <-current.closed:
		return true
	default:
		// A qualification exchange is intentionally bounded to a very small
		// ACCEPTED -> terminal lifecycle. Overflow is consumed here so an
		// untrusted device cannot redirect an unbound test result into the
		// production command path.
		return true
	}
}
