# F-020 Extension Startup Recovery Notes

Safety invariants for this slice:

1. Extension startup recovery may restore component liveness only.
2. `ReconcileBounded` delegates each attempt to the existing `RuntimeSupervisor.Reconcile` implementation.
3. A retry is authorized only when every terminal leaf error from a reconciliation pass is `ErrRuntimeProbeHandshake`.
4. A permanent failure in any enabled extension prevents another global reconciliation attempt in that pass.
5. No recovery attempt changes Runtime Snapshot, Session authority, Cue execution identity, Stage Device command identity, or Companion execution identity.
6. Reconnect/restart never implies `replay_allowed=true`.
