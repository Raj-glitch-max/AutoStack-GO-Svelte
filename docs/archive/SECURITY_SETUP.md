# AutoStack Security Setup Guide

## Critical Security Fixes Applied

This document outlines the critical security fixes that have been applied to AutoStack and the required setup steps.

### 1. Encryption Key Management (CRITICAL)

**Problem**: The encryption key was being regenerated on every container restart, making all previously encrypted AWS credentials unreadable.

**Fix**: Encryption key is now persisted in `.env` file and Kubernetes Secret.

**Setup Steps**:

#### For Docker Compose:

```bash
# Generate encryption key (only run once!)
./scripts/generate-encryption-key.sh

# This creates a .env file with AUTOSTACK_ENCRYPTION_KEY
# NEVER change this key after first deployment
```

#### For Kubernetes:

```bash
# Generate a key
KEY=$(openssl rand -base64 32)

# Update the secret
kubectl create secret generic autostack-secrets \
  --from-literal=AUTOSTACK_ENCRYPTION_KEY="$KEY" \
  -n one-click

# Or edit deployment/secret.yaml and apply:
# Replace REPLACE_WITH_YOUR_BASE64_KEY with your generated key
kubectl apply -f deployment/secret.yaml
```

**IMPORTANT**: 
- Back up your encryption key securely
- If lost, all encrypted AWS credentials become unrecoverable
- Never commit `.env` to version control (already in .gitignore)

### 2. Multi-Tenancy Authorization (CRITICAL)

**Problem**: Collections had no server-side authorization rules, allowing any authenticated user to access other users' data via direct API calls.

**Fix**: Added PocketBase collection rules that enforce `user = @request.auth.id` on:
- `projects` collection
- `deployments` collection  
- `rollouts` collection
- `awsDeployments` collection (already had rules)
- `awsCredentials` collection (already had rules)

**Migration**: Run automatically on next PocketBase startup via migration `1776000000_add_authorization_rules.js`

### 3. Kubernetes RBAC Scoping (CRITICAL)

**Problem**: AutoStack service account had `cluster-admin` role with unlimited permissions across the entire cluster.

**Fix**: Created scoped `autostack-operator` ClusterRole with only required permissions:
- Namespace management
- Pod/Service/ConfigMap/Secret operations within managed namespaces
- Deployment/Job/CronJob management
- Ingress/Storage read access
- No ability to modify cluster-wide RBAC or other critical resources

**Apply**: 
```bash
kubectl apply -f deployment/clusterrole.yaml
kubectl apply -f deployment/clusterrolebinding.yaml
```

### 4. Terraform State Persistence

**Problem**: Terraform state was stored on ephemeral container filesystem, causing state loss on pod restart.

**Fix**: Added PersistentVolumeClaim for `/app/terraform-workdir`

**Apply**:
```bash
kubectl apply -f deployment/pvc-terraform.yaml
```

### 5. Frontend Import Fix

**Problem**: Intelligence components imported `{ pb }` which doesn't exist - should be `{ client }`.

**Fix**: Updated imports in:
- `frontend/src/lib/components/intelligence/ErrorAnalysis.svelte`
- `frontend/src/lib/components/intelligence/RecoveryDashboard.svelte`
- `frontend/src/routes/intelligence/+page.svelte`

## Verification Checklist

After applying these fixes:

- [ ] Encryption key is set and persisted (check `.env` or K8s secret)
- [ ] PocketBase migrations run successfully
- [ ] Test that users cannot access other users' projects via API
- [ ] Verify AutoStack pod has limited permissions (not cluster-admin)
- [ ] Terraform state persists across pod restarts
- [ ] Intelligence pages load without console errors

## Remaining Security Considerations

### High Priority (Not Yet Fixed):
1. **WebSocket Authentication**: Log/event streams need JWT validation
2. **Rate Limiting**: No per-user deployment quotas or API rate limits
3. **Cost Guards**: No pre-deployment cost validation or spending limits

### Medium Priority:
4. **Terraform State Backend**: Consider S3 backend instead of local PVC
5. **Secrets Rotation**: No mechanism for rotating encryption keys
6. **Audit Logging**: No audit trail for sensitive operations

## Emergency Recovery

### If Encryption Key is Lost:
1. All encrypted AWS credentials are unrecoverable
2. Users must re-enter their AWS credentials
3. Truncate `awsCredentials` collection: `DELETE FROM awsCredentials;`

### If Unauthorized Access Detected:
1. Check PocketBase collection rules are applied
2. Verify migration `1776000000_add_authorization_rules.js` ran
3. Check PocketBase logs for direct API access patterns

### If Cluster Compromise:
1. Verify AutoStack is using `autostack-operator` role, not `cluster-admin`
2. Check for unexpected ClusterRoleBindings: `kubectl get clusterrolebindings`
3. Audit namespace creation: `kubectl get namespaces -l one-click.dev/managed=true`

## Contact

For security issues, please report privately to the maintainers.
