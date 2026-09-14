# F-020 Extension Startup Recovery Scope Freeze

This slice ends at bounded Hub-startup extension runtime restoration.

Out of scope for this slice:

- automatic restart after a runtime crashes while the Hub is already serving operators;
- Companion reconnect loops;
- Stage Device reconnect loops;
- retry or replay of Cue/Action/device commands;
- operator controls for self-healing policy;
- Raspberry Pi physical qualification.

Those concerns belong to later F-020 slices after this startup path is software-qualified.
