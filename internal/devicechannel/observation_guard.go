package devicechannel

import (
	"context"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

// observeCurrentDevice serializes a transport observation with registration
// and disconnection. An authenticated but displaced socket MUST NOT overwrite
// the replacement's ONLINE state or network diagnostics. This applies to
// both legacy v1 and experimental v2. Authentication and schema validation
// are performed separately by the receive path.
func (r *Runtime) observeCurrentDevice(
	ctx context.Context, current *connection,
	observation deviceexperience.RuntimeObservation,
	network deviceexperience.NetworkObservation,
) (bool, error) {
	if r == nil || current == nil {
		return false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.connections[current.deviceID] != current {
		return false, nil
	}
	select {
	case <-current.closed:
		return false, nil
	default:
	}
	if _, err := r.repository.ObserveDevice(ctx, observation); err != nil {
		return true, err
	}
	_, err := r.repository.RecordNetworkObservation(ctx, network)
	return true, err
}

// probeV2AfterReconnect is an OPTIONAL software diagnostic, never a cue,
// runtime.ready, snapshot activation or output command. The opt-in flag and
// capability negotiation are independent and both required. The callback is
// bounded and fenced to the *exact* socket that initiated it.
func (r *Runtime) probeV2AfterReconnect(current *connection) {
	if r == nil || !r.autoProbeV2 || current == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), liveLightingProbeTimeout+time.Second)
		defer cancel()
		_, _ = r.probeV2SoftwareLevelsForConnection(ctx, current.deviceID, current)
	}()
}
