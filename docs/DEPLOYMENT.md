# AutoStack Deployment Guide

Production deployment instructions for AutoStack.

## Pre-Deployment Checklist

- [ ] API keys configured (Infracost, Resend)
- [ ] Encryption key generated (32 bytes)
- [ ] Database backups configured
- [ ] Monitoring setup (optional)
- [ ] SSL certificates ready
- [ ] Domain configured

## Deployment Options

### Option 1: Docker (Recommended)

```bash
# Build image
docker build -t autostack:latest .

# Run container
docker run -d \
  -p 8090:8090 \
  -e INFRACOST_API_KEY=your_key \
  -e RESEND_API_KEY=your_key \
  -e AUTOSTACK_ENCRYPTION_KEY=your_key \
  -v $(pwd)/pb_data:/app/pb_data \
  --name autostack \
  autostack:latest
```

### Option 2: Kubernetes

```bash
# Create secrets
kubectl create secret generic autostack-secrets \
  --from-literal=INFRACOST_API_KEY=your_key \
  --from-literal=RESEND_API_KEY=your_key \
  --from-literal=AUTOSTACK_ENCRYPTION_KEY=your_key

# Deploy
kubectl apply -f deployment/

# Check status
kubectl get pods -n autostack
```

### Option 3: Docker Compose

```bash
# Production compose file
docker compose -f docker-compose.prod.yml up -d
```

## Environment Configuration

### Production Environment Variables

```bash
# API Keys
INFRACOST_API_KEY=your_production_key
RESEND_API_KEY=your_production_key

# Security
AUTOSTACK_ENCRYPTION_KEY=your_32_byte_key

# Database
DATABASE_URL=postgresql://user:pass@host:5432/autostack

# Optional: Monitoring
SENTRY_DSN=your_sentry_dsn
LOGTAIL_TOKEN=your_logtail_token
```

## Security Considerations

### API Key Management

1. **Never commit API keys to version control**
2. Use secrets manager (AWS Secrets Manager, HashiCorp Vault)
3. Rotate keys quarterly
4. Monitor API usage for anomalies

### SSL/TLS

```bash
# Using Let's Encrypt with Certbot
certbot certonly --standalone -d your-domain.com
```

### Firewall Rules

```bash
# Allow only necessary ports
ufw allow 80/tcp   # HTTP
ufw allow 443/tcp  # HTTPS
ufw allow 22/tcp   # SSH (from specific IPs only)
ufw enable
```

## Monitoring

### Health Checks

```bash
# Application health
curl http://localhost:8090/api/health

# Database health
curl http://localhost:8090/api/health/db
```

### Logging

```bash
# View logs
docker logs -f autostack

# Or with kubectl
kubectl logs -f deployment/autostack -n autostack
```

### Metrics

Monitor these metrics:
- API response times
- Error rates
- Database connections
- Memory usage
- CPU usage

## Backup Strategy

### Database Backups

```bash
# Automated daily backups
0 2 * * * /usr/local/bin/backup-autostack.sh
```

### Backup Script

```bash
#!/bin/bash
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backups/autostack"
mkdir -p $BACKUP_DIR

# Backup database
cp /app/pb_data/data.db $BACKUP_DIR/data_$DATE.db

# Backup uploads
tar -czf $BACKUP_DIR/uploads_$DATE.tar.gz /app/pb_data/storage

# Keep only last 30 days
find $BACKUP_DIR -name "*.db" -mtime +30 -delete
find $BACKUP_DIR -name "*.tar.gz" -mtime +30 -delete
```

## Scaling

### Horizontal Scaling

```yaml
# kubernetes deployment
replicas: 3
```

### Database Scaling

Consider PostgreSQL for production:
- Better performance at scale
- Advanced features (replication, partitioning)
- Better concurrent write handling

## Rollback Procedure

```bash
# Docker
docker stop autostack
docker run -d --name autostack autostack:previous-version

# Kubernetes
kubectl rollout undo deployment/autostack -n autostack
```

## Troubleshooting

### Application Won't Start

```bash
# Check logs
docker logs autostack

# Check environment variables
docker exec autostack env | grep -E "INFRACOST|RESEND"
```

### High Memory Usage

```bash
# Check memory
docker stats autostack

# Restart if needed
docker restart autostack
```

### Database Locked

```bash
# Check for long-running queries
# Restart application
docker restart autostack
```

## Support

For production issues:
- Check logs first
- Review [TROUBLESHOOTING.md](./TROUBLESHOOTING.md)
- Contact support: support@autostack.io
