# Environment Matrix

## Environments

### Local Development
Used for feature work and safe iteration.

Typical needs:
- frontend running locally
- backend running locally
- PocketBase locally or in a dev container
- mock provider endpoints where possible
- local Kubernetes or kind/minikube for operator testing when needed

### Staging
Used to validate the integration between UI, backend, provider logic, and cloud APIs before production use.

Staging should include:
- real auth configuration if possible
- real provider validation against non-production accounts
- non-production pricing / cost checks
- audit logging verification
- rollback and delete tests

### Production
The live user-facing environment.

Production expectations:
- no destructive defaults
- strict logging hygiene
- secure credential storage
- monitored reconciliation loops
- no schema experiments

## Configuration Categories

- application URLs
- backend/frontend ports
- PocketBase location
- operator namespace / cluster access
- cloud provider credentials
- registry credentials
- pricing API settings
- AI provider settings
- notification provider settings
- audit / retention settings
- feature flags

## Env Var Principles

- Use a consistent prefix convention for AutoStack-specific values.
- Keep environment names explicit.
- Do not assume defaults for security-sensitive settings.
- Add new variables to this matrix before relying on them in code.

## Stability Rule

If a setting affects:
- network access
- credentials
- pricing
- secret handling
- destructive operations

then it belongs here and should be documented before implementation.
