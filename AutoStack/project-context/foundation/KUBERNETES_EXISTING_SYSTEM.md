# Kubernetes Existing System — Protected Baseline

> **This system already works. Treat it as production.**
>
> The multicloud effort must be strictly additive. No schema breakage, no operator rewrites, no CRD changes, no “cleanup refactors” on the Kubernetes path.

## What This System Already Does

- Deploys containerized applications to Kubernetes clusters
- Maps each project to a namespace
- Uses the `one-click.dev/v1alpha1 Rollout` CRD as the Kubernetes source of truth
- Reconciles the CRD into `Deployment`, `Service`, `Ingress`, `HPA`, `PVC`, and related resources
- Streams logs, events, and metrics to the frontend in real time
- Tracks rollout history and supports rollback
- Polls registries for image updates using semver and timestamp policies
- Supports reusable blueprints and blueprint sharing
- Masks secret values in the UI after entry

## Invariants That Must Not Change

1. The CRD schema remains frozen unless a formal migration plan is approved.
2. The Kubernetes operator reconciliation loop is not modified without explicit instruction.
3. Existing PocketBase collections used by Kubernetes remain intact.
4. The frontend status vocabulary stays unchanged:
   - `pending`
   - `deploying`
   - `running`
   - `failed`
   - `rolled_back`
5. Cloud features never replace the Kubernetes path.
6. The frontend does not need to know whether an event originated from Kubernetes or a cloud provider.

## Operational Contract

- Kubernetes remains the anchor system.
- Cloud deployment flows must feel identical from the user’s perspective.
- If a change might affect the Kubernetes path, stop and isolate it first.
- Any cloud work belongs behind new abstractions, new collections, or new services.

## Immediate Implication for Multicloud

The Kubernetes deployment system is the stable reference implementation. Every cloud provider integration should mimic its user experience, not rewrite it.
