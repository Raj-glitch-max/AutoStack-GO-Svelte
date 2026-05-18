# AutoStack Platform — Release Engineering

## Version Policy

AutoStack uses semantic versioning: `vMAJOR.MINOR.PATCH`

| Change Type                                   | Version Bump |
|-----------------------------------------------|--------------|
| Breaking API or schema change                 | MAJOR        |
| New capability, backward-compatible           | MINOR        |
| Bug fix, security patch, documentation        | PATCH        |

**Current release: v7.0.0** — Production Readiness, Security Hardening, Chaos Validation

---

## Release Checklist

Before tagging any release, the following must be true:

### Code Quality
- [ ] `go build ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0 with 0 failures
- [ ] Race detector clean: `go test -race ./pkg/security/... ./pkg/persistence/... ./pkg/loadtest/...`

### Security
- [ ] No `fmt.Println` in production code (CI enforces via grep)
- [ ] No credentials or tokens in any log statement (manual review + CI grep)
- [ ] `pkg/security` tests all pass
- [ ] Rate limiter defaults reviewed for production fitness
- [ ] RBAC matrix reviewed — no unintended permission grants

### Replay and Certification
- [ ] `pkg/replay` tests pass with count=3 (determinism verified across runs)
- [ ] `pkg/certification` hash stability verified across runs
- [ ] Platform certification produces `PlatformCertified=true` in smoke run

### Backup and Restore
- [ ] Backup manifest hash verification passes
- [ ] `RestorePlatformSnapshot()` refuses tampered payloads
- [ ] At least one full backup/restore cycle exercised in staging

### Observability
- [ ] `pkg/metrics` snapshot returns correct values after mutations
- [ ] Health summary thresholds match SLO targets in `docs/slo.md`

### Chaos
- [ ] All 10 chaos fault types produce `EvidenceIntact=true`
- [ ] Faults requiring operator action correctly set `RecoveryDetected=false`

### Deployment
- [ ] `deploy/docker/Dockerfile.platform-api` builds successfully
- [ ] `deploy/helm/autostack/` Helm chart validates: `helm lint`
- [ ] `docker-compose.yaml` starts cleanly with `/healthz` returning 200

---

## Release Manifest

Each release produces a `release-manifest.json` at the repository root of the release tag. Fields:

```json
{
  "version": "v7.0.0",
  "schema_version": "7.0.0",
  "platform_claim": "A deterministic orchestration operating platform with replay-certified execution, append-only operational evidence, governance-enforced automation, and production-grade survivability guarantees.",
  "packages": ["pkg/security", "pkg/chaos", "pkg/metrics", "pkg/backup", "pkg/loadtest"],
  "certification_gates": 5,
  "readiness_gates": 10,
  "released_at": "<RFC3339 UTC>",
  "release_hash": "<SHA-256 of stable fields — released_at excluded>"
}
```

---

## Changelog — v7.0.0

### Added
- `pkg/security` — JWT auth, RBAC (5 roles, 20+ permissions), tenant isolation, rate limiting, append-only hash-chained audit trail, secret reference store (36 tests)
- `pkg/chaos` — deterministic fault injection for 10 fault types; evidence-invariant validation (13 tests)
- `pkg/metrics` — in-process counters, gauges, histograms, platform health summary (15 tests)
- `pkg/backup` — backup manifests (content-identity hash), payload integrity verification, `RestorePlatformSnapshot()` with hash-before-restore guard (13 tests), expanded 10-gate production readiness audit (16 tests)
- `pkg/loadtest` — concurrent load scenarios validating replay determinism, RBAC correctness, counter accuracy, hash uniqueness (8 tests)
- `.github/workflows/platform-hardening.yml` — build+vet, unit tests, race detector, replay determinism, append-only validation, security hardening, benchmarks, docker build
- `docs/slo.md` — replay, contradiction scan, certification latency SLOs; RTO/RPO objectives

### Security
- Constant-time API token verification (`hmac.Equal`)
- Default-deny RBAC matrix — no permission granted unless explicitly listed
- SHA-256 tenant isolation hash — cross-tenant access detectable
- Credentials never logged at any log level (CI grep enforced)
- Hash-chained audit trail — tampering detectable

### Invariants Preserved
- Append-only evidence: no delete path anywhere in storage or audit trail
- Replay determinism: same inputs → same ManifestHash, verified with count=3
- Default-deny: governance and auth both deny unless explicitly granted
- Evidence over automation: worker crashes and governance delays require operator action

### Known Limitations
- Rate limiter is in-process only (not distributed across replicas)
- Chaos engine simulates fault outcomes; it does not inject real network or storage failures
- Load tests are in-process; they do not measure real cloud provider latency
- SLO latency targets are based on in-process benchmarks, not production traffic

---

## Release Signature

Releases are signed via `git tag -s v7.0.0`. Verify with:

```sh
git verify-tag v7.0.0
```

The `release-manifest.json` release hash excludes `released_at` (content-identity).
