# AutoStack

A user-friendly platform for deploying containerized applications on Kubernetes with a single click.

## Introduction

AutoStack provides a clean interface for deploying Docker containers on a Kubernetes cluster without needing to write complex Kubernetes manifests.

Built with:
- **Frontend**: [SvelteKit](https://kit.svelte.dev/)
- **Backend**: [Go](https://golang.org/) & [PocketBase](https://pocketbase.io/)
- **Kubernetes Operator**: [Operator SDK](https://sdk.operatorframework.io/)

## Features

- Deploy any Docker image to Kubernetes in seconds
- Blueprint system for reusable deployment templates
- Real-time deployment status and logs
- Rollout history and rollback support
- Auto-update images on a schedule
- Project-based organization (maps to Kubernetes namespaces)

## Getting Started

### Prerequisites

- Docker Desktop with Kubernetes enabled
- kubectl configured

### Run with Docker Compose

```sh
docker compose up
```

Access the app at `http://localhost:8090`

## License

MIT
