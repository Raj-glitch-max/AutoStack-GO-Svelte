# Deployment Model

## Mental Model

A deployment is a desired application state mapped to one execution target.

The target can be:
- Kubernetes
- AWS ECS Fargate
- Google Cloud Run
- Azure ACA
- future provider types

## Core Objects

- **Project**: organizational container
- **Deployment / Rollout**: application intent
- **Target**: where the deployment runs
- **Blueprint**: reusable template
- **History Record**: immutable version snapshot
- **Cloud Account**: provider access configuration

## Kubernetes Model

Kubernetes remains the current mature execution path.
The existing CRD / operator flow remains unchanged.

## Cloud Model

Cloud deployments should:
- be defined in PocketBase
- be reconciled by a backend service
- map to provider-native resources
- expose the same UI vocabulary as Kubernetes
- maintain history and rollback capability

## Deployment Lifecycle

1. user creates or selects a template
2. user configures image, resources, env, secrets, and target
3. system validates credentials and target readiness
4. system estimates cost
5. user deploys
6. reconciler creates or updates resources
7. backend streams synthesized status
8. history is recorded
9. rollback re-applies prior desired state

## Cost Model

Cost estimation should be split into:
- compute
- network
- storage
- load balancing
- logging / monitoring
- provider-specific overhead

The model should admit uncertainty where exact usage is unknown.

## Workload Scope

Start with stateless workloads first unless a stateful story is explicitly modeled.
Stateful support, service dependencies, multi-region routing, and promotion pipelines are larger concerns and should be designed consciously rather than assumed.

## Product Stance

AutoStack is a control plane and deployment orchestrator, not a general-purpose cloud builder.
It should make common deployment paths easy, safe, and understandable.
