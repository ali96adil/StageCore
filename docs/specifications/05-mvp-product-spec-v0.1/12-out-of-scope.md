# 12 — Out of Scope

The following are intentionally excluded from MVP v0.1. They are not rejected product ideas; they are deferred so the first StageCore can actually be built and validated.

## Hardware & Real-Time Expansion

- physical StageCore Node products;
- Relay/Sensor/DMX/Wireless Node firmware;
- distributed offline Node rules;
- certified motor/safety controller integration beyond generic future capability contracts;
- full native DMX lighting-console engine.

## Advanced Show Features

- automatic Adapt Show to Venue workflow;
- full logical-lighting patch editor and venue scaling UI;
- projector camera calibration / computer vision;
- automated projection mapping;
- AI rehearsal analysis or cue recommendations;
- actor tracking;
- automatic execution based on AI predictions.

## Media & Archive Expansion

- video playback engine;
- live streaming media from Hub to projector;
- heavy transcoding;
- performance-recording ingest UI;
- multi-camera archive management;
- full Project Vault lifecycle/archive verification;
- automatic NAS/cloud backup workflows.

The data model may reserve clean extension points, but these features must not create MVP implementation dependencies.

## Operations & Scale

- production-grade High Availability Hub cluster;
- automatic Hub failover;
- active-active Machine Roles;
- advanced multi-operator conflict resolution;
- cloud control dependency;
- remote Internet show control;
- enterprise identity providers;
- complex VLAN/router management UI.

## Plugin Ecosystem

The MVP needs a Plugin/Adapter foundation sufficient for OSC and later basic HTTP/MIDI. It does not need a public marketplace, third-party signing ecosystem, automated plugin store, or compatibility certification program.

## Safety Boundary

StageCore MVP is not a certified life-safety system and does not replace E-Stops, interlocks, certified motion controllers, fire systems, or other required safety hardware.

## Scope Change Rule

An out-of-scope feature can enter MVP only if all three are true:

1. the core end-to-end loop already passes;
2. the feature is required to validate a real MVP use case;
3. adding it does not postpone the defined MVP acceptance gate.

Otherwise it belongs to a post-MVP milestone.
