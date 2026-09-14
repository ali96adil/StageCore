# F-020 Extension Startup Recovery Acceptance

Software acceptance for this slice requires:

- `RuntimeSupervisor.ReconcileBounded` is used by Hub startup;
- retry budget is bounded to 3 attempts;
- retry occurs only for transient runtime handshake failures;
- integrity, permission, isolation, configuration, and mixed permanent failures fail closed;
- caller cancellation/deadline stops further attempts;
- no Cue/Action or device command replay is introduced;
- Core CI passes tests, vet, race tests, and Linux ARM64 CGo-free product builds.

Physical qualification is intentionally deferred under Issue #148.
