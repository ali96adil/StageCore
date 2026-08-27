package storagehealth

import "path/filepath"

type AggregateStatus struct {
	State    State
	DataRoot Status
	VaultRoot Status
}

type Monitor struct {
	policy    *Policy
	dataRoot  string
	vaultRoot string
}

func NewMonitor(policy *Policy, dataRoot, vaultRoot string) *Monitor {
	if policy == nil {
		policy = NewPolicy(0, 0)
	}
	return &Monitor{policy: policy, dataRoot: filepath.Clean(dataRoot), vaultRoot: filepath.Clean(vaultRoot)}
}

func (m *Monitor) Status() AggregateStatus {
	if m == nil || m.policy == nil {
		unavailable := Status{State: Unavailable, Reason: "storage monitor is unavailable"}
		return AggregateStatus{State: Unavailable, DataRoot: unavailable, VaultRoot: unavailable}
	}
	data := m.policy.Probe(m.dataRoot)
	vault := m.policy.Probe(m.vaultRoot)
	return AggregateStatus{State: worseState(data.State, vault.State), DataRoot: data, VaultRoot: vault}
}

func worseState(a, b State) State {
	if stateRank(b) > stateRank(a) {
		return b
	}
	return a
}

func stateRank(state State) int {
	switch state {
	case Healthy:
		return 0
	case Warning:
		return 1
	case Degraded:
		return 2
	case Critical:
		return 3
	case Unavailable:
		return 4
	default:
		return 4
	}
}
