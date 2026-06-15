# ADR-034: PostgreSQL production quorum boundary

## Status

Accepted.

## Context

NexusIM has a local PostgreSQL HA smoke based on `bitnamilegacy/postgresql-repmgr + pgpool`. It is useful for development and interview demos because the stable pgpool writer endpoint can survive a primary stop and the IM chain can continue after failover.

The local quorum observation smoke also proved an important limit: when both standby containers are stopped, the same local topology still accepts writes on the remaining primary. That means the current local setup is not quorum-fenced and must not be described as production-grade split-brain protection.

## Decision

The current `repmgr + pgpool` topology is accepted only as a local smoke and development topology. It is not the production PostgreSQL HA decision.

Any production PostgreSQL HA design for NexusIM must provide an explicit quorum and fencing boundary before it can be accepted. The implementation is not fixed to one product, but it must prove these properties:

- a single writer lease or primary election backed by an external quorum, managed HA contract or equivalent fencing mechanism;
- a documented behavior for primary isolation, standby isolation, network partition and quorum loss;
- fail-closed or explicitly degraded write behavior when safe primary ownership cannot be proven;
- a recovery and failback procedure that prevents split-brain writes from being silently merged;
- observable leader, replication, lag, fencing and failover state;
- backup, restore and point-in-time recovery procedures separate from HA failover;
- a clear RPO / RTO claim with smoke or drill evidence.

Acceptable future candidates include managed PostgreSQL HA, a Patroni-style DCS/quorum design, a Kubernetes operator with documented fencing semantics, or another design that satisfies the same evidence. The ADR does not require choosing one today.

## Required Evidence Before Production Claim

Before any document or interview material calls PostgreSQL "production HA", the project must include:

- an ADR updating the selected topology and ownership boundary;
- a local or staged partition drill for primary isolation and quorum loss;
- a failover drill proving client write behavior through the service stack;
- a split-brain prevention or fencing proof, not just successful failover;
- a failback / rejoin runbook for the old primary and stale replicas;
- backup restore and PITR smoke evidence;
- alert rules and dashboard panels for leader state, replication lag and unsafe quorum;
- a statement of known non-goals, such as in-flight transaction continuity if it is not guaranteed.

## Consequences

Local `repmgr + pgpool` smoke remains useful, but it is only evidence for local stable-writer behavior. The quorum observation result is now a deliberate gap marker: NexusIM can explain exactly why the current local topology is not enough, what production properties are missing, and what evidence is required before claiming production PostgreSQL HA.
