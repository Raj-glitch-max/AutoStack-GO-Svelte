# API Contracts

## Contract Principles

- The frontend consumes stable JSON shapes.
- Backend behavior should be predictable and versioned where necessary.
- Status values should remain consistent across Kubernetes and cloud targets.
- Cloud-specific provider details should not leak into the UI contract.

## Core Status Vocabulary

- `pending`
- `deploying`
- `running`
- `failed`
- `rolled_back`

## WebSocket Message Shapes

### log_line
- timestamp
- stream
- text

### metrics_update
- cpu_percent
- memory_mb
- timestamp

### deployment_event
- rollout id
- phase
- message
- timestamp

### pod_event / service_event / ingress_event / job_event
- resource reference
- status or state
- message
- timestamp

Cloud providers must synthesize compatible events for the frontend.

## High-Level API Areas

### Deployments
- create deployment
- update deployment
- rollback deployment
- delete deployment
- fetch history

### Cloud Accounts
- add account
- validate account
- rotate credentials
- list supported regions
- surface connection errors clearly

### Cost
- estimate before deploy
- refresh after deploy
- compare estimated vs actual where possible

### Observability
- fetch logs
- stream logs
- fetch metrics
- stream metrics
- expose deployment health

### Blueprints
- list templates
- create from template
- share template
- version template

## Contract Rule

If the frontend already understands a message or status today, cloud paths should reuse the same shape instead of inventing a second vocabulary.
