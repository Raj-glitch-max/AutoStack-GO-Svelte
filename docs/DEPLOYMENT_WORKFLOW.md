# AutoStack Deployment Workflow — Quick Reference

## Daily Development Workflow

### 1. Start New Feature

```bash
# Ensure you're on develop and up to date
git checkout develop
git pull origin develop

# Create feature branch
git checkout -b feature/my-awesome-feature

# Make your changes
# ... code, code, code ...

# Commit and push
git add .
git commit -m "feat: add awesome feature"
git push origin feature/my-awesome-feature
```

### 2. Create Pull Request

1. Go to GitHub repository
2. Click "Pull requests" → "New pull request"
3. Base: `develop` ← Compare: `feature/my-awesome-feature`
4. Fill in PR description
5. Click "Create pull request"

**What happens automatically:**
- ✅ CI runs: backend tests, frontend tests, linting
- ✅ Docker build test (no push)
- ✅ Terraform validation
- ✅ Security scans

### 3. After PR Approval

```bash
# Merge via GitHub UI (Squash and merge recommended)
```

**What happens automatically:**
- ✅ Code merges to `develop`
- ✅ Docker image builds and pushes to ghcr.io
- ✅ Auto-deploys to **STAGING** environment
- ✅ Slack notification (if configured)

### 4. Test in Staging

```bash
# Check staging deployment
kubectl get pods -n autostack-staging
kubectl logs -n autostack-staging deployment/one-click

# Access staging URL
# https://staging.autostack.example.com
```

### 5. Promote to Production

```bash
# Create PR from develop to main
git checkout develop
git pull origin develop
git checkout main
git pull origin main
git checkout -b release/v1.2.0
git merge develop
git push origin release/v1.2.0
```

1. Create PR: Base: `main` ← Compare: `release/v1.2.0`
2. Get approval from team lead
3. Merge PR

**What happens automatically:**
- ✅ Docker image builds with `:latest` tag
- ✅ Waits for manual approval (production environment gate)
- ✅ After approval: Deploys to **PRODUCTION**
- ✅ Slack notification

### 6. Create Release (Optional)

```bash
# Tag the release
git checkout main
git pull origin main
git tag v1.2.0
git push origin v1.2.0
```

**What happens automatically:**
- ✅ Builds multi-arch Docker image (amd64 + arm64)
- ✅ Creates GitHub Release with changelog
- ✅ Attaches compiled binaries

## Quick Commands

### Check CI Status

```bash
# View workflow runs
gh run list

# View specific run
gh run view <run-id>

# Watch live run
gh run watch
```

### Check Deployments

```bash
# Staging
kubectl get all -n autostack-staging

# Production
kubectl get all -n autostack-production
```

### View Logs

```bash
# Staging logs
kubectl logs -f -n autostack-staging deployment/one-click

# Production logs
kubectl logs -f -n autostack-production deployment/one-click
```

### Rollback Deployment

```bash
# Staging
kubectl rollout undo deployment/one-click -n autostack-staging

# Production
kubectl rollout undo deployment/one-click -n autostack-production
```

### Manual Deployment

```bash
# Get latest image tag
IMAGE_TAG=$(git rev-parse --short HEAD)

# Deploy to staging
kubectl set image deployment/one-click \
  one-click=ghcr.io/raj-glitch-max/autostack-go-svelte:sha-${IMAGE_TAG} \
  -n autostack-staging

# Deploy to production (requires approval)
kubectl set image deployment/one-click \
  one-click=ghcr.io/raj-glitch-max/autostack-go-svelte:sha-${IMAGE_TAG} \
  -n autostack-production
```

## Environment URLs

| Environment | URL | Branch | Auto-Deploy |
|-------------|-----|--------|-------------|
| Staging | https://staging.autostack.example.com | `develop` | ✅ Yes |
| Production | https://autostack.example.com | `main` | ⚠️ Requires approval |

## Branch Strategy

```
feature/* → develop → main → v1.2.3
              ↓        ↓       ↓
           STAGING  PRODUCTION RELEASE
```

## Workflow Status Badges

Add these to your README.md:

```markdown
![CI](https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/workflows/CI/badge.svg)
![CD](https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/workflows/CD/badge.svg)
![Security](https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/workflows/Security%20Scan/badge.svg)
```

## Troubleshooting

### CI Fails on PR

1. Check workflow logs in GitHub Actions
2. Run tests locally:
   ```bash
   # Backend
   cd pocketbase && go test ./...
   
   # Frontend
   cd frontend && npm run check && npm run lint
   ```
3. Fix issues and push again

### Deployment Fails

1. Check workflow logs
2. Check Kubernetes events:
   ```bash
   kubectl get events -n autostack-staging --sort-by='.lastTimestamp'
   ```
3. Check pod status:
   ```bash
   kubectl describe pod -n autostack-staging <pod-name>
   ```

### Image Not Found

1. Verify image exists:
   ```bash
   docker pull ghcr.io/raj-glitch-max/autostack-go-svelte:latest
   ```
2. Check GitHub Packages page
3. Verify GITHUB_TOKEN permissions

## Emergency Procedures

### Hotfix to Production

```bash
# Create hotfix branch from main
git checkout main
git pull origin main
git checkout -b hotfix/critical-bug

# Fix the bug
# ... code ...

# Commit and push
git add .
git commit -m "fix: critical bug in production"
git push origin hotfix/critical-bug

# Create PR to main (skip develop)
# After approval and merge, it auto-deploys to production
```

### Rollback Production

```bash
# Option 1: Kubernetes rollback
kubectl rollout undo deployment/one-click -n autostack-production

# Option 2: Deploy previous image
kubectl set image deployment/one-click \
  one-click=ghcr.io/raj-glitch-max/autostack-go-svelte:sha-<previous-commit> \
  -n autostack-production
```

### Stop Deployment

```bash
# Cancel GitHub Actions workflow
gh run cancel <run-id>

# Or via GitHub UI: Actions → Click run → Cancel workflow
```

## Best Practices

1. **Always create feature branches** - Never commit directly to develop or main
2. **Write descriptive commit messages** - Use conventional commits (feat:, fix:, docs:)
3. **Test locally before pushing** - Run tests and linting
4. **Keep PRs small** - Easier to review and less risky
5. **Update documentation** - Keep docs in sync with code changes
6. **Monitor deployments** - Check logs after deployment
7. **Use semantic versioning** - v1.2.3 (major.minor.patch)

## Commit Message Convention

```
feat: add new feature
fix: fix bug in component
docs: update documentation
style: format code
refactor: refactor module
test: add tests
chore: update dependencies
```

## Need Help?

- 📖 Full setup guide: `docs/CI_CD_SETUP.md`
- 🔧 Troubleshooting: Check workflow logs in GitHub Actions
- 💬 Team chat: #autostack-deployments Slack channel
- 🚨 Emergency: Contact DevOps team lead

## Useful Links

- GitHub Repository: https://github.com/Raj-glitch-max/AutoStack-GO-Svelte
- GitHub Actions: https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/actions
- Container Registry: https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkgs/container/autostack-go-svelte
- Security Alerts: https://github.com/Raj-glitch-max/AutoStack-GO-Svelte/security

---

**Remember**: The pipeline is your friend. It catches bugs before they reach production! 🚀
