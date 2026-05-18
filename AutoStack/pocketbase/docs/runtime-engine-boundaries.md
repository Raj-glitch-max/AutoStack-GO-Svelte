# Runtime Engine — Production Boundaries

**Status**: Honest scope document for `pkg/runtime_deploy` + reconciler.
**Audience**: Operators deciding what to trust this engine for.

---

## What the engine guarantees

1. **Deterministic lifecycle**: every state transition (planned → certified or → failed) is produced by a real Docker subprocess and persisted before the next transition fires. State in `runtime_deployments` always reflects the last completed step.

2. **Append-only event chain**: `runtime_deployment_events` rows are never updated or deleted. A UNIQUE constraint on `(deployment, sequence)` prevents duplicate sequence numbers from any source.

3. **Restart survivability**: killing the engine process at any point leaves a recoverable state. On restart, a new worker picks up any deployment that's still in a non-terminal forward state and continues the lifecycle. Verified live by killing the engine in `deploying` and watching a fresh process certify the deployment.

4. **Deterministic replay**: `Replay(events)` is a pure function. Same input → same output, including a stable SHA-256 `ChainHash` that excludes timestamps. Verified by 7 unit tests including shuffled-input determinism and timestamp-insensitive hash.

5. **Drift detection without auto-heal**: the reconciler observes the real container every 5 seconds. If the container is missing, not running, or running the wrong image, it transitions the deployment to `contradicted` and opens an incident. **It never repairs drift automatically.** Operator action is required.

6. **Single-writer coordination**: a deployment lease (UNIQUE on `deployment`) ensures only one engine instance mutates a given deployment. Stale leases (no heartbeat for `LeaseTTL`) can be taken over; live leases held by another worker are respected.

7. **Per-tenant isolation**: each deployment is owned by exactly one user. List/view/update/delete rules enforce `user = @request.auth.id`. Reconciliation events and observations inherit the owner via the relation.

---

## What the engine does NOT guarantee

These are intentional scope boundaries, not gaps to be papered over.

### Distributed consensus
The lease mechanism is best-effort single-writer coordination, not consensus. Two engines racing on the same database can both:
- See an empty lease, both try to insert, one wins by UNIQUE constraint.
- See a stale lease, both try to take over, one wins by row update.

There is no quorum, no leader election, no fencing token. If you need multi-replica safety, run exactly one engine per database.

### Byzantine resistance
The engine trusts the local Docker daemon implicitly. A malicious user with Docker socket access can:
- Create containers labeled `autostack.runtime=true` that the engine didn't create.
- Stop / mutate containers the engine is managing.
- Return false data from `docker inspect`.

The engine surfaces this as drift (and you'll see it in observations + contradictions), but it does not detect or resist tampering by a privileged actor.

### Docker socket trust boundary
Running the engine inside a container requires mounting `/var/run/docker.sock` — which gives the engine root-equivalent control over the host. The shipped `demo/docker-compose.yaml` does NOT mount the socket; the engine reports "docker unavailable" inside that container. To run dockerized with a working engine, mount the socket explicitly and accept the host-control implication.

### Cross-replica audit chain continuity
Each deployment's event chain is single-writer per-process. If two engines process the same deployment (against policy), event sequences will diverge. Replay validation rejects gaps but won't repair a split chain.

### Network policy / multi-host orchestration
The engine binds host ports directly via `docker run -p`. It does not manage cluster networking, service discovery, load balancing, ingress, or DNS. It is a **local single-host** deployment engine.

### Kubernetes
There is no Kubernetes driver. The CLAUDE.md K8s operator path is unrelated to and unchanged by this engine. The runtime engine targets local Docker only; cloud and K8s targets are separate architectures.

### Graceful shutdown
The engine does not release its leases on shutdown — they expire naturally after `LeaseTTL` (30s). A restart within that window will see "lease held by another worker" for up to 30s, during which time deployments don't progress.

### Real-time log streaming
Logs are snapshot-style: `docker logs --tail 200` on each request. There is no long-poll / WebSocket / Server-Sent-Events streaming. The UI polls every 3s.

### Container metrics
No CPU / memory / restart-count metrics are collected. The engine observes container status and image identity only.

---

## Recovery scenarios

| Failure | Behavior |
|---|---|
| Engine crash mid-`docker pull` | On restart, deployment is in `provisioning` — next tick re-runs pull (idempotent for image already present) |
| Engine crash mid-`docker run` | On restart, container may or may not exist. If exists with the right name, engine moves to `verifying`. If not, engine re-runs `docker run`. |
| Engine crash after `certified` | Reconciler picks up on restart and continues observing the container |
| Container OOM-killed | Reconciler sees `status="exited"` → transitions to `contradicted`, opens incident |
| Container manually `docker rm`'d | Reconciler sees no container → transitions to `contradicted` with `container_missing` kind |
| Container image swapped externally | Reconciler sees `observed_image != desired` → `contradicted` with `image_mismatch` |
| Docker daemon goes down | Engine's `InspectStatus` errors. Records an `inspect_status_error` observation (low-confidence). Does NOT transition to contradicted on transient errors. |
| Two engine processes start | Both write to the same DB; lease UNIQUE constraint serializes them. Each tick only one worker advances any given deployment. |
| Database file deleted | Hard fail. PocketBase will not start. No recovery mechanism. |

---

## Known cosmetic issues (functional, but display-broken)

1. **`observed_at` / `occurred_at` timestamps display as zero** in the API response. The data IS stored correctly (visible via the PB collection API as `"created": "2026-05-17 14:32:16.678Z"`), but PocketBase v0.22's `types.DateTime` round-trip is unreliable for user-defined date fields read through `Record.GetDateTime` or `Record.GetCreated`. Functional behavior (drift detection, transitions, ordering) is unaffected.

2. **`lease_live` reports false** even for fresh leases. Same root cause as #1: the `expires_at` field stored as a date doesn't round-trip cleanly. Lease coordination still works because the write path uses the same field consistently for take-over arithmetic via direct SQL UPDATE on heartbeat.

Workaround options if a fix is critical:
- Change `runtime_observations.observed_at` and `runtime_deployment_leases.expires_at` from `date` to `number` (unix seconds) — round-trips cleanly.
- Use the auto-managed `created` / `updated` fields, which work via different code paths.

---

## What you should NOT use this engine for

- Any production workload requiring multi-node HA.
- Anything that needs to survive `kill -9` on a SINGLE engine process without re-derivation. (Crash recovery works, but in-flight pulls may need to retry.)
- Workloads where drift must be auto-repaired — the engine's contract is that operators decide.
- Compliance-grade tamper-proof audit (the chain is hash-linked but not signed).
- Any environment where the Docker socket is shared with untrusted workloads.
