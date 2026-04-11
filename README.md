# AutoStack

A user-friendly platform for deploying containerized applications on Kubernetes and AWS with intelligent cost estimation and AI-powered error recovery.

## Introduction

AutoStack provides a clean interface for deploying Docker containers on Kubernetes clusters and AWS infrastructure without needing to write complex manifests or Terraform code.

Built with:
- **Frontend**: [SvelteKit](https://kit.svelte.dev/)
- **Backend**: [Go](https://golang.org/) & [PocketBase](https://pocketbase.io/)
- **Infrastructure**: Kubernetes & AWS with Terraform
- **Intelligence**: AI-powered error analysis and recovery

## Features

- 🚀 Deploy any Docker image to Kubernetes in seconds
- ☁️ AWS deployment with Terraform integration
- 💰 Real-time AWS cost estimation and monitoring
- 🤖 AI-powered error analysis and automatic recovery
- 📦 Blueprint system for reusable deployment templates
- 📊 Real-time deployment status and logs
- 🔄 Rollout history and rollback support
- ⏰ Auto-update images on a schedule
- 🏗️ Project-based organization (maps to Kubernetes namespaces)
- 🔐 Secure credential management for AWS

## Getting Started

### Prerequisites

- Docker Desktop with Kubernetes enabled
- kubectl configured
- (Optional) AWS account for cloud deployments

### Run with Docker Compose

```sh
docker compose up
```

Access the app at `http://localhost:8090`

## Documentation

- [User Guide](USER_GUIDE.md)
- [AWS Cost Estimation](docs/AWS_COST_ESTIMATION_USER_GUIDE.md)
- [Intelligence System](pocketbase/pkg/intelligence/README.md)
- [Project Documentation](PROJECT_DOCUMENTATION.md)

## Author

**Raj Patil** - [@Raj-glitch-max](https://github.com/Raj-glitch-max)

## License

Apache License 2.0 - see [LICENSE](LICENSE) file for details
