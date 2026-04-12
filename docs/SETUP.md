# AutoStack Setup Guide

Complete setup instructions for AutoStack.

## Prerequisites

- Docker Desktop with Kubernetes enabled
- kubectl configured
- Go 1.23+ (for development)
- Node.js 18+ (for frontend development)

## Quick Start

### 1. Clone and Configure

```bash
git clone https://github.com/Raj-glitch-max/AutoStack.git
cd AutoStack
cp .env.example .env
```

### 2. Configure API Keys

Edit `.env` and add your API keys:

```bash
# Required for AWS cost estimation
INFRACOST_API_KEY=your_infracost_key

# Required for email notifications
RESEND_API_KEY=your_resend_key
RESEND_FROM_EMAIL=onboarding@resend.dev

# Required for encryption
AUTOSTACK_ENCRYPTION_KEY=$(openssl rand -base64 32)
```

### 3. Run with Docker Compose

```bash
docker compose up
```

Access at: http://localhost:8090

## Development Setup

### Backend (Go + PocketBase)

```bash
cd pocketbase
go mod download
go build -o autostack
./autostack serve
```

### Frontend (SvelteKit)

```bash
cd frontend
npm install
npm run dev
```

## Configuration

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `INFRACOST_API_KEY` | Infracost API key for cost estimation | Yes |
| `RESEND_API_KEY` | Resend API key for emails | Yes |
| `AUTOSTACK_ENCRYPTION_KEY` | 32-byte encryption key | Yes |
| `ADMIN_EMAIL` | Initial admin email | No |
| `ADMIN_PASSWORD` | Initial admin password | No |

### API Keys

**Infracost** (Cost Estimation)
- Sign up: https://www.infracost.io/
- Free tier: 100 estimates/month
- Paid: $50/month unlimited

**Resend** (Email Notifications)
- Sign up: https://resend.com/
- Free tier: 3,000 emails/month
- Paid: $20/month for 50,000 emails

## Deployment

See [DEPLOYMENT.md](./DEPLOYMENT.md) for production deployment instructions.

## Troubleshooting

### Build Errors

```bash
cd pocketbase
go mod tidy
go build
```

### Port Already in Use

```bash
# Change port in docker-compose.yaml or kill existing process
lsof -ti:8090 | xargs kill -9
```

### Database Issues

```bash
# Reset database (WARNING: deletes all data)
rm pocketbase/pb_data/data.db
./autostack serve
```

## Next Steps

- Read [USER_GUIDE.md](./USER_GUIDE.md) for usage instructions
- Check [API.md](./api/API.md) for API documentation
- See [CONTRIBUTING.md](../CONTRIBUTING.md) for development guidelines
