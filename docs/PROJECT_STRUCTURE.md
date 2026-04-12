# AutoStack Project Structure

Complete overview of the AutoStack project organization.

## Directory Structure

```
AutoStack/
├── .github/                    # GitHub workflows and templates
├── .kiro/                      # Kiro AI configuration
├── deployment/                 # Kubernetes deployment manifests
│   ├── deployment.yaml        # Main deployment
│   ├── service.yaml           # Service definition
│   ├── ingress.yaml           # Ingress rules
│   └── ...                    # Other K8s resources
├── docs/                       # Documentation
│   ├── api/                   # API documentation
│   │   └── API.md            # REST API reference
│   ├── guides/                # User guides (future)
│   ├── archive/               # Old documentation
│   ├── SETUP.md              # Setup instructions
│   ├── DEPLOYMENT.md         # Deployment guide
│   └── PROJECT_STRUCTURE.md  # This file
├── frontend/                   # SvelteKit frontend
│   ├── src/                   # Source code
│   │   ├── lib/              # Shared libraries
│   │   │   ├── api/          # API clients
│   │   │   ├── components/   # Svelte components
│   │   │   ├── stores/       # Svelte stores
│   │   │   └── utils/        # Utility functions
│   │   ├── routes/           # SvelteKit routes
│   │   └── app.html          # HTML template
│   ├── static/                # Static assets
│   ├── package.json           # NPM dependencies
│   └── svelte.config.js       # Svelte configuration
├── pocketbase/                 # Go backend
│   ├── pkg/                   # Go packages
│   │   ├── aws/              # AWS integration
│   │   ├── controller/       # HTTP controllers
│   │   ├── cost/             # Cost calculation
│   │   ├── intelligence/     # AI error recovery
│   │   ├── k8s/              # Kubernetes client
│   │   ├── notifications/    # Email service
│   │   ├── terraform/        # Terraform executor
│   │   └── ...               # Other packages
│   ├── pb_data/              # PocketBase data
│   │   ├── data.db          # SQLite database
│   │   └── storage/         # File uploads
│   ├── pb_migrations/        # Database migrations
│   ├── templates/            # Terraform templates
│   ├── go.mod                # Go dependencies
│   └── main.go               # Entry point
├── scripts/                    # Utility scripts
│   └── generate-encryption-key.sh
├── .dockerignore              # Docker ignore rules
├── .editorconfig              # Editor configuration
├── .env.example               # Environment template
├── .gitignore                 # Git ignore rules
├── .pre-commit-config.yaml    # Pre-commit hooks
├── CODE_OF_CONDUCT.md         # Code of conduct
├── CODEOWNERS                 # Code ownership
├── CONTRIBUTING.md            # Contribution guidelines
├── docker-compose.yaml        # Docker Compose config
├── Dockerfile                 # Docker image definition
├── LICENSE                    # Apache 2.0 license
├── README.md                  # Project overview
├── SECURITY.md                # Security policy
└── USER_GUIDE.md              # User documentation
```

## Key Directories

### `/pocketbase/pkg/`

Go packages organized by functionality:

- **aws/** - AWS integration (Infracost, credentials)
- **cache/** - Caching layer for cost estimates
- **controller/** - HTTP request handlers
- **cost/** - Cost calculation and estimation
- **crypto/** - Encryption utilities
- **env/** - Environment configuration
- **health/** - Health check endpoints
- **image/** - Docker image utilities
- **intelligence/** - AI error analysis and recovery
- **jobs/** - Background job processing
- **k8s/** - Kubernetes client and operations
- **middleware/** - HTTP middleware
- **models/** - Data models
- **notifications/** - Email and notification service
- **shutdown/** - Graceful shutdown handling
- **startup/** - Startup validation
- **terraform/** - Terraform execution
- **util/** - Utility functions
- **validation/** - Input validation
- **watcher/** - WebSocket event watchers

### `/frontend/src/lib/components/`

Svelte components organized by feature:

- **admin/** - Admin panel components
- **architecture/** - Architecture visualization
- **aws/** - AWS-specific components
- **base/** - Base/shared components
- **blueprints/** - Blueprint selection
- **cost/** - Cost estimation UI
- **deployments/** - Deployment management
- **intelligence/** - AI recovery UI
- **projects/** - Project management
- **rollouts/** - Rollout history

### `/deployment/`

Kubernetes manifests for production deployment:

- **ns.yaml** - Namespace definition
- **deployment.yaml** - Application deployment
- **service.yaml** - Service definition
- **ingress.yaml** - Ingress rules
- **configmap.yaml** - Configuration
- **secret.yaml** - Secrets template
- **pvc.yaml** - Persistent volume claims
- **serviceaccount.yaml** - Service account
- **clusterrole.yaml** - RBAC cluster role
- **clusterrolebinding.yaml** - RBAC binding

## File Naming Conventions

### Go Files

- `*_service.go` - Service implementations
- `*_controller.go` - HTTP controllers
- `*_test.go` - Unit tests
- `*_integration_test.go` - Integration tests

### Svelte Files

- `*.svelte` - Svelte components
- `+page.svelte` - SvelteKit pages
- `+layout.svelte` - Layout components
- `+server.ts` - Server-side endpoints

### Configuration Files

- `.env.example` - Environment template
- `*.yaml` - Kubernetes/Docker configs
- `*.json` - JSON configuration
- `*.toml` - TOML configuration

## Import Paths

### Go Imports

```go
import (
    "github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
    "github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/cost"
    "github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/intelligence"
)
```

### TypeScript Imports

```typescript
import { pb } from '$lib/pocketbase';
import { CostEstimator } from '$lib/components/cost';
import { estimateCost } from '$lib/api/cost';
```

## Configuration Files

### Backend Configuration

- **go.mod** - Go module definition
- **main.go** - Application entry point
- **.env** - Environment variables (not in git)
- **.env.example** - Environment template

### Frontend Configuration

- **package.json** - NPM dependencies
- **svelte.config.js** - Svelte configuration
- **vite.config.ts** - Vite configuration
- **tailwind.config.cjs** - Tailwind CSS config
- **tsconfig.json** - TypeScript configuration

### Deployment Configuration

- **docker-compose.yaml** - Local development
- **Dockerfile** - Container image
- **deployment/*.yaml** - Kubernetes manifests

## Data Storage

### Database

- **Location**: `pocketbase/pb_data/data.db`
- **Type**: SQLite
- **Migrations**: `pocketbase/pb_migrations/`

### File Uploads

- **Location**: `pocketbase/pb_data/storage/`
- **Organization**: By collection and record ID

### Terraform State

- **Location**: S3 bucket (user-provided)
- **Path**: `tfstate/{userID}/{deploymentID}/terraform.tfstate`
- **Locking**: DynamoDB table

## Build Artifacts

### Backend

- **Binary**: `pocketbase/autostack`
- **Size**: ~100MB
- **Platform**: Linux/macOS/Windows

### Frontend

- **Build**: `frontend/build/`
- **Assets**: Static HTML, CSS, JS
- **Size**: ~5MB

### Docker

- **Image**: `autostack:latest`
- **Size**: ~150MB
- **Base**: golang:1.23-alpine

## Environment Variables

See `.env.example` for complete list:

- `INFRACOST_API_KEY` - Infracost API key
- `RESEND_API_KEY` - Resend API key
- `AUTOSTACK_ENCRYPTION_KEY` - Encryption key (32 bytes)
- `ADMIN_EMAIL` - Initial admin email
- `ADMIN_PASSWORD` - Initial admin password

## Development Workflow

1. **Backend**: `cd pocketbase && go run main.go serve`
2. **Frontend**: `cd frontend && npm run dev`
3. **Database**: Auto-created in `pb_data/`
4. **Migrations**: Auto-applied on startup

## Testing

### Backend Tests

```bash
cd pocketbase
go test ./...                    # All tests
go test ./pkg/cost/...          # Specific package
go test -race ./...             # Race detection
go test -cover ./...            # Coverage
```

### Frontend Tests

```bash
cd frontend
npm test                        # All tests
npm run test:coverage          # With coverage
```

### Integration Tests

```bash
cd pocketbase
go run test_integration.go     # Integration tests
```

## Documentation

- **User Docs**: `USER_GUIDE.md`, `docs/SETUP.md`
- **API Docs**: `docs/api/API.md`
- **Dev Docs**: `CONTRIBUTING.md`, `docs/PROJECT_STRUCTURE.md`
- **Archive**: `docs/archive/` (old documentation)

## Version Control

### Ignored Files

See `.gitignore`:
- `.env` (contains secrets)
- `pb_data/` (database and uploads)
- `node_modules/` (NPM packages)
- `build/` (build artifacts)
- `*.log` (log files)

### Tracked Files

- Source code (`.go`, `.svelte`, `.ts`)
- Configuration (`.yaml`, `.json`)
- Documentation (`.md`)
- Templates (`.tf`)

## Deployment Targets

### Local Development

- Docker Compose
- Kubernetes (Docker Desktop)

### Production

- Kubernetes cluster
- AWS ECS/Fargate
- Docker Swarm

## Security

### Sensitive Files

- `.env` - Never commit
- `pb_data/data.db` - Contains user data
- `pb_data/storage/` - Contains uploads

### Encrypted Data

- AWS credentials (AES-256)
- User passwords (bcrypt)
- Session tokens (JWT)

## Maintenance

### Regular Tasks

- Update dependencies (`go get -u`, `npm update`)
- Review security advisories
- Backup database
- Rotate API keys
- Clean old logs

### Monitoring

- Application logs: `pocketbase/logs/`
- Error tracking: Sentry (optional)
- Metrics: Better Stack (optional)

## Support

For questions about project structure:
- Check this document
- See `CONTRIBUTING.md`
- Ask in GitHub Discussions
