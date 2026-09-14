# F-020 Extension Startup Recovery Test Matrix

| Case | Expected result |
| --- | --- |
| First startup probe handshake fails, next attempt succeeds | Runtime becomes `READY` within bounded retry budget |
| Runtime artifact integrity failure | Fail immediately; no retry |
| Mixed handshake + permanent error across enabled extensions | Fail closed; no retry |
| Recovery context cancelled or deadline expires | No additional attempt starts |
| Successful extension already restored before another retry pass | Existing process is preserved by normal `Reconcile` semantics |
| Hub recovery pass | No Cue/Action or transport command replay |

The PR CI is the software gate. Raspberry Pi and production-device qualification remains deferred under Issue #148.
