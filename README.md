# AutoStack

**Smart Cloud Deployment Platform with AI-Powered Cost Estimation**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.24+-00ADD8.svg)](https://golang.org/)
[![Node Version](https://img.shields.io/badge/node-20+-339933.svg)](https://nodejs.org/)
[![CI](https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/workflows/CI/badge.svg)](https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/actions/workflows/ci.yml)
[![CD](https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/workflows/CD/badge.svg)](https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/actions/workflows/cd.yml)
[![Security](https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/workflows/Security%20Scan/badge.svg)](https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/actions/workflows/security.yml)

AutoStack simplifies cloud deployments with real-time cost estimation, AI-powered error recovery, and one-click infrastructure provisioning.

## ✨ Features

- 🚀 **One-Click Deployments** - Deploy to Kubernetes or AWS in seconds
- 💰 **Real-Time Cost Estimation** - See AWS costs before deploying (powered by Infracost)
- 🤖 **AI Error Recovery** - Automatic detection and recovery from deployment failures
- 📧 **Smart Notifications** - Email alerts for cost overruns and deployment status
- 📦 **Blueprint System** - Reusable templates for common architectures
- 🔐 **Secure by Default** - Encrypted credentials and user isolation
- 📊 **Live Monitoring** - Real-time logs and deployment status
- 🔄 **Production CI/CD** - Automated testing, security scanning, and deployment

## 🚀 Quick Start

### Prerequisites

- Docker Desktop with Kubernetes enabled
- kubectl configured
- (Optional) AWS account for cloud deployments

### Run with Docker Compose

```bash
git clone https://github.com/Raj-glitch-max/AutoStack.git
cd AutoStack
cp .env.example .env
# Edit .env with your API keys
docker compose up
```

Access at: **http://localhost:8090**

### Configuration

Get your free API keys:
- **Infracost** (cost estimation): https://www.infracost.io/
- **Resend** (email notifications): https://resend.com/

Add to `.env`:
```bash
INFRACOST_API_KEY=your_key_here
RESEND_API_KEY=your_key_here
AUTOSTACK_ENCRYPTION_KEY=$(openssl rand -base64 32)
```

## 📚 Documentation

- **[Setup Guide](docs/SETUP.md)** - Complete installation instructions
- **[User Guide](USER_GUIDE.md)** - How to use AutoStack
- **[API Documentation](docs/api/API.md)** - REST API reference
- **[Deployment Guide](docs/DEPLOYMENT.md)** - Production deployment
- **[Contributing](CONTRIBUTING.md)** - Development guidelines

## 🏗️ Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│  SvelteKit  │────▶│  PocketBase  │────▶│ Kubernetes  │
│  Frontend   │     │   + Go API   │     │     or      │
└─────────────┘     └──────────────┘     │     AWS     │
                            │             └─────────────┘
                            ▼
                    ┌──────────────┐
                    │  Infracost   │
                    │   Resend     │
                    └──────────────┘
```

## 💡 Use Cases

### Static Website
Deploy a static site to S3 + CloudFront for ~$2/month

### Web Application
Deploy a full web app with Fargate + RDS for ~$32/month

### Microservices
Deploy multiple services with API Gateway + SQS for ~$50/month

## 🛠️ Technology Stack

**Frontend:**
- SvelteKit
- TailwindCSS
- Flowbite

**Backend:**
- Go 1.24+
- PocketBase (SQLite)
- Kubernetes Client
- Terraform

**Infrastructure:**
- Kubernetes
- AWS (Fargate, RDS, S3, CloudFront)
- Terraform

**APIs:**
- Infracost (cost estimation)
- Resend (email notifications)

## 🔄 CI/CD Pipeline

AutoStack uses a production-grade CI/CD pipeline with GitHub Actions:

### Workflows

- **CI**: Automated testing, linting, and validation on every PR
- **CD**: Automated deployment to staging (develop) and production (main)
- **Security**: Daily vulnerability scanning with Trivy, gosec, and npm audit
- **Terraform**: Validation of infrastructure templates
- **Release**: Automated releases with multi-arch Docker images

### Deployment Strategy

```
feature/* → develop → main → v1.2.3
              ↓        ↓       ↓
           STAGING  PRODUCTION RELEASE
```

### Quick Links

- 📖 [CI/CD Setup Guide](docs/CI_CD_SETUP.md) - Complete setup instructions
- 🚀 [Deployment Workflow](docs/DEPLOYMENT_WORKFLOW.md) - Developer quick reference
- 📋 [Implementation Summary](docs/CI_CD_IMPLEMENTATION_SUMMARY.md) - Technical details

### Features

- ✅ Automated testing and linting
- ✅ Docker image building and pushing to ghcr.io
- ✅ Kubernetes deployment with rollout verification
- ✅ Environment-based approval gates
- ✅ Slack notifications
- ✅ Security scanning and vulnerability detection
- ✅ Automated releases with changelogs

## 📊 Cost Transparency

AutoStack shows you **exact costs before deployment**:

| Blueprint | Monthly Cost | Services |
|-----------|--------------|----------|
| Static Website | ~$2 | S3, CloudFront, Route53 |
| Web App | ~$32 | Fargate, ALB, RDS |
| Full Stack | ~$53 | Fargate, RDS, S3, CloudFront |
| Microservices | ~$50+ | Multiple Fargate services, SQS |

*Costs are estimates and may vary based on usage*

## 🤝 Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development Workflow

1. Fork the repository
2. Create a feature branch from `develop`
3. Make your changes
4. Run tests: `go test ./...` and `npm run check`
5. Create a pull request to `develop`
6. CI will automatically run tests and checks

See [Deployment Workflow](docs/DEPLOYMENT_WORKFLOW.md) for detailed instructions.

## 📝 License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

## 🙏 Acknowledgments

- [Infracost](https://www.infracost.io/) - Real-time AWS pricing
- [Resend](https://resend.com/) - Email delivery
- [PocketBase](https://pocketbase.io/) - Backend framework
- [SvelteKit](https://kit.svelte.dev/) - Frontend framework

## 📞 Support

- **Documentation**: [docs/](docs/)
- **CI/CD Setup**: [docs/CI_CD_SETUP.md](docs/CI_CD_SETUP.md)
- **Issues**: [GitHub Issues](https://github.com/Raj-glitch-max/AutoStack/issues)
- **Discussions**: [GitHub Discussions](https://github.com/Raj-glitch-max/AutoStack/discussions)

## 🗺️ Roadmap

- [x] Kubernetes deployments
- [x] AWS deployments with Terraform
- [x] Real-time cost estimation
- [x] AI-powered error recovery
- [x] Email notifications
- [ ] Multi-cloud support (GCP, Azure)
- [ ] Team collaboration features
- [ ] Advanced monitoring and analytics
- [ ] Custom domain management
- [ ] Automated backups

---

**Made with ❤️ by [Raj Patil](https://github.com/Raj-glitch-max)**
