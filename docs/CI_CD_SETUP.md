# AutoStack — Production CI/CD Pipeline Setup Guide

## Overview

This document provides complete instructions for setting up the production-grade CI/CD pipeline for AutoStack using GitHub Actions.

## Architecture

The pipeline consists of 5 workflows:

1. **CI** (`ci.yml`) - Runs on every PR and push to main/develop
2. **CD** (`cd.yml`) - Deploys to staging (develop) and production (main)
3. **Security** (`security.yml`) - Daily security scans
4. **Terraform Validate** (`terraform-validate.yml`) - Validates Terraform templates
5. **Release** (`release.yml`) - Creates releases from version tags

## Prerequisites

- GitHub repository with admin access
- Kubernetes clusters for staging and production
- kubectl configured with access to both clusters
- Slack workspace (optional, for notifications)

## Step 1: Configure GitHub Repository Settings

### 1.1 Actions Permissions

1. Go to: **Settings → Actions → General**
2. Set "Actions permissions" to: **Allow all actions and reusable workflows**
3. Set "Workflow permissions" to: **Read and write permissions**
4. Check: **Allow GitHub Actions to create and approve pull requests**

### 1.2 GitHub Container Registry

The pipeline uses GitHub Container Registry (ghcr.io) which requires no additional setup. The `GITHUB_TOKEN` automatically has push permissions when "Read and write permissions" is enabled.

### 1.3 Create Environments

Go to: **Settings → Environments** and create:

#### Environment: `staging`
- No protection rules
- Add environment secret: `KUBECONFIG_STAGING`

#### Environment: `production`
- Protection rule: **Required reviewers** (add at least one reviewer)
- Add environment secret: `KUBECONFIG_PRODUCTION`

### 1.4 Repository Secrets

Go to: **Settings → Secrets and variables → Actions → New repository secret**

Add these secrets:

| Secret Name | Description | How to Get It |
|------------|-------------|---------------|
| `KUBECONFIG_STAGING` | Base64-encoded kubeconfig for staging | `cat ~/.kube/config \| base64 -w 0` |
| `KUBECONFIG_PRODUCTION` | Base64-encoded kubeconfig for production | `cat ~/.kube/config \| base64 -w 0` |
| `SLACK_WEBHOOK_URL` | Slack incoming webhook URL (optional) | Create in Slack App → Incoming Webhooks |

### 1.5 Branch Protection Rules

#### Rule for `main` branch:

Go to: **Settings → Branches → Add rule**

- Branch name pattern: `main`
- ✅ Require a pull request before merging
- ✅ Require status checks to pass before merging
  - Required checks: `backend`, `frontend`, `terraform-validate`
- ✅ Require branches to be up to date before merging
- ✅ Do not allow bypassing the above settings

#### Rule for `develop` branch:

- Branch name pattern: `develop`
- ✅ Require status checks to pass before merging
  - Required checks: `backend`, `frontend`

## Step 2: Prepare Kubernetes Clusters

### 2.1 Create Namespaces

Run these commands for each cluster:

```bash
# For staging cluster
kubectl create namespace autostack-staging --dry-run=client -o yaml | kubectl apply -f -

# For production cluster
kubectl create namespace autostack-production --dry-run=client -o yaml | kubectl apply -f -
```

### 2.2 Verify Deployment Manifests

Check that `deployment/deployment.yaml` has the correct namespace or will accept namespace override:

```yaml
metadata:
  name: one-click
  namespace: one-click  # This will be overridden by -n flag in kubectl
```

The CD workflow uses `kubectl apply -f deployment/ -n autostack-production` which will apply the correct namespace.

## Step 3: Verify Frontend Configuration

The frontend already has the required scripts in `package.json`:

```json
{
  "scripts": {
    "check": "svelte-kit sync && svelte-check --tsconfig ./tsconfig.json",
    "lint": "prettier --check . && eslint .",
    "build": "vite build"
  }
}
```

If these are missing, add them and ensure eslint and prettier are installed:

```bash
cd frontend
npm install -D eslint eslint-plugin-svelte prettier prettier-plugin-svelte
```

## Step 4: Git Branching Strategy

The pipeline enforces this Git flow:

```
feature/your-feature
        │
        ▼ PR → review → merge
     develop  ──────────────────→  auto-deploy to STAGING
        │
        ▼ PR → review → merge
      main    ──────────────────→  deploy to PRODUCTION (requires approval)
        │
        ▼ git tag v1.2.3
     release  ──────────────────→  GitHub Release + multi-arch Docker image
```

### Workflow:

1. Create feature branch from `develop`: `git checkout -b feature/my-feature develop`
2. Make changes and push: `git push origin feature/my-feature`
3. Create PR to `develop` → CI runs automatically
4. After review and merge → Auto-deploys to staging
5. Create PR from `develop` to `main`
6. After review and merge → Deploys to production (requires approval)
7. Tag release from `main`: `git tag v1.0.0 && git push origin v1.0.0`

## Step 5: Verification Checklist

### GitHub Actions UI

- [ ] Go to **Actions** tab — all 5 workflows appear
- [ ] Trigger `ci.yml` by pushing to develop
- [ ] All 4 CI jobs pass: backend, frontend, terraform-validate, docker-build
- [ ] `security.yml` can be triggered manually

### Docker Images

- [ ] Push to develop → Image appears at `ghcr.io/{owner}/{repo}:develop`
- [ ] Push to main → Image appears at `ghcr.io/{owner}/{repo}:latest`
- [ ] Image tags include `sha-{7-char-commit}` format

### Kubernetes Deployments

- [ ] Push to develop → `deploy-staging` job runs
- [ ] `kubectl get pods -n autostack-staging` shows running pods
- [ ] Push to main → GitHub shows production environment gate
- [ ] After approval → `deploy-production` runs

### Security Scans

- [ ] Go to **Security → Code scanning** — Trivy and gosec results appear
- [ ] Gitleaks scan passes (no secrets in repo history)

## Step 6: Common Issues and Fixes

### Issue: `go: cannot find module`

**Cause**: `go.sum` is out of sync

**Fix**:
```bash
cd pocketbase
go mod tidy
git add go.sum
git commit -m "Update go.sum"
```

### Issue: `npm ci fails`

**Cause**: `package-lock.json` out of sync

**Fix**:
```bash
cd frontend
npm install
git add package-lock.json
git commit -m "Update package-lock.json"
```

### Issue: `kubectl: unauthorized`

**Cause**: Kubeconfig secret expired or wrong cluster

**Fix**:
```bash
# Generate new kubeconfig
cat ~/.kube/config | base64 -w 0

# Update secret in GitHub Settings → Secrets
```

### Issue: `Docker push: denied`

**Cause**: Workflow permissions not set to read/write

**Fix**: Go to **Settings → Actions → General** → set "Read and write permissions"

### Issue: `golangci-lint: no such file`

**Cause**: Version mismatch

**Fix**: Check Go version compatibility in `.github/workflows/ci.yml`

### Issue: `Trivy action fails`

**Cause**: Missing `security-events: write` permission

**Fix**: Verify permissions block in `security.yml` job

### Issue: `Release workflow doesn't trigger`

**Cause**: Tag not pushed

**Fix**: Always push tags explicitly:
```bash
git tag v1.0.0
git push origin v1.0.0
```

## Step 7: Slack Notifications (Optional)

To enable Slack notifications:

1. Create a Slack App: https://api.slack.com/apps
2. Enable Incoming Webhooks
3. Add webhook URL to repository secrets as `SLACK_WEBHOOK_URL`
4. Notifications will appear for:
   - ✅ Successful staging deployments
   - ✅ Successful production deployments
   - ❌ Failed deployments

## Step 8: Monitoring and Maintenance

### Daily Tasks

- Check **Actions** tab for failed workflows
- Review **Security** tab for new vulnerabilities
- Monitor Slack notifications (if enabled)

### Weekly Tasks

- Review and merge dependabot PRs
- Check Docker image sizes
- Review deployment logs

### Monthly Tasks

- Update workflow action versions
- Review and update branch protection rules
- Audit access to secrets and environments

## Security Best Practices

1. **Never commit secrets** - Use GitHub Secrets for all sensitive data
2. **Rotate kubeconfig regularly** - Update every 90 days
3. **Review security scan results** - Address CRITICAL and HIGH vulnerabilities
4. **Use environment protection** - Require approvals for production
5. **Audit access logs** - Review who deployed what and when

## Troubleshooting

### Workflow Logs

View detailed logs:
1. Go to **Actions** tab
2. Click on failed workflow run
3. Click on failed job
4. Expand failed step to see error details

### Kubernetes Logs

Check deployment status:
```bash
# Staging
kubectl get pods -n autostack-staging
kubectl logs -n autostack-staging deployment/one-click

# Production
kubectl get pods -n autostack-production
kubectl logs -n autostack-production deployment/one-click
```

### Docker Image Issues

Pull and inspect image:
```bash
docker pull ghcr.io/{owner}/{repo}:latest
docker run -it ghcr.io/{owner}/{repo}:latest /bin/sh
```

## Support

For issues with the CI/CD pipeline:

1. Check this documentation
2. Review workflow logs in GitHub Actions
3. Check Kubernetes cluster logs
4. Review recent commits for breaking changes

## Appendix: Workflow Triggers

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| CI | Push to main/develop, PRs | Validate code quality |
| CD | Push to main/develop | Deploy to environments |
| Security | Push to main, daily at 2 AM UTC | Security scanning |
| Terraform Validate | Changes to templates | Validate IaC |
| Release | Push version tag (v*.*.*) | Create GitHub release |

## Appendix: Environment Variables

The following environment variables are used in the pipeline:

| Variable | Source | Used In |
|----------|--------|---------|
| `GITHUB_TOKEN` | Automatic | All workflows |
| `KUBECONFIG_STAGING` | Secret | CD (staging) |
| `KUBECONFIG_PRODUCTION` | Secret | CD (production) |
| `SLACK_WEBHOOK_URL` | Secret | CD (notifications) |
| `NODE_ENV` | Hardcoded | Frontend build |
| `CGO_ENABLED` | Hardcoded | Go build |

## Conclusion

This CI/CD pipeline provides:

- ✅ Automated testing and validation
- ✅ Secure deployment to staging and production
- ✅ Daily security scanning
- ✅ Automated releases with changelogs
- ✅ Slack notifications
- ✅ Branch protection and code review enforcement

Follow this guide carefully to ensure a smooth setup and reliable deployments.
