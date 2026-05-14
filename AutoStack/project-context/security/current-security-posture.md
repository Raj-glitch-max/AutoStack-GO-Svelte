# Security Posture - Current State

## Last Updated
2025-05-13

## Kubernetes System (UNCHANGED)

The Kubernetes deployment system maintains its existing security posture:
- No changes to operator RBAC
- No changes to credential handling
- No changes to authentication

## New Cloud Features

### What Was Implemented

1. **Credential Storage**
   - Cloud credentials stored in PocketBase `cloud_accounts.credentials_encrypted`
   - Field is JSON blob (encrypted at rest by PocketBase or application-level AES)
   - Credentials never returned in API responses (masked display only)

2. **Error Sanitization**
   - `sanitizeError()` function in reconciler redacts sensitive patterns
   - Patterns: credential, secret, token, key, password, private_key, service_account, access_key, api_key
   - Error messages limited to 500 chars

3. **Access Control**
   - Cloud account rules: `user = @request.auth.id` (user-scoped)
   - API routes require authentication via `apis.RequireRecordAuth("users")`

### What Is Missing / Incomplete

1. **Encryption Key Management (ISSUE-002)**
   - Status: CRITICAL - Open
   - Currently using `AUTOSTACK_ENCRYPTION_KEY` environment variable
   - Not integrated with cloud KMS
   - Impact: If key leaks, all stored credentials compromised

2. **Credential Validation Logging**
   - Validation successes/failures not written to audit log
   - Should track who validated, when, result

3. **API Key Scopes**
   - Cloud account operations not yet integrated with API key system
   - Need to add scopes: cloud_account:read, cloud_account:write, cost:read

4. **Dual Authorization**
   - Not implemented for cloud account management
   - Per SECURITY_AND_ACCESS.md, enterprise feature

## Security Rules From CLAUDE.md (Preserved)

1. **Credentials never leave encrypted home**
   - Encrypted at rest, decrypted only in-memory during use
   - Never in logs (enforced via sanitization)

2. **Minimum permissions**
   - Provider implementations request minimum cloud permissions needed
   - No billing modification rights
   - No IAM administration rights

3. **Every action attributable**
   - Audit logging needed (ISSUE-004) - not yet implemented

4. **Workload isolation**
   - Customer workloads not in same security context as AutoStack
   - Credentials stored encrypted

## Security-Relevant Code Locations

| File | Security Concern |
|------|------------------|
| `pkg/providers/cloudrun/provider.go` | Credentials decrypted at runtime |
| `pkg/reconciler/cloud.go` | Error sanitization implemented |
| `pkg/controller/cloudAccounts.go` | Access control via rules |
| `pb_migrations/1715300000_created_cloud_accounts.js` | credentials_encrypted field |

## Cloud Provider Permissions (AWS Example - from SECURITY_AND_ACCESS.md)

AutoStack requires:
- ECS: CreateService, UpdateService, DeleteService, DescribeServices, etc.
- EC2: DescribeVpcs, DescribeSubnets, DescribeSecurityGroups
- ELBv2: CreateLoadBalancer, CreateTargetGroup, etc.
- CloudWatchLogs: CreateLogGroup, PutLogEvents
- ECR: GetAuthorizationToken, BatchCheckLayerAvailability

NOT required:
- iam:* - no IAM management
- s3:* - no S3 access
- rds:* - no database management
- ec2:TerminateInstances - no instance termination

## Security Verification Needed

1. [ ] Encryption at rest works correctly
2. [ ] Credentials never appear in logs
3. [ ] API responses don't expose credentials
4. [ ] Access control rules enforced
5. [ ] Error messages sanitized
6. [ ] Audit logging implemented for cloud operations

## Known Security Risks

1. **High**: Encryption key from env var (ISSUE-002)
2. **Medium**: No audit logging for cloud operations
3. **Medium**: No rate limiting on cloud API calls
4. **Low**: Hardcoded pricing not from live API (could mislead on costs)

## Recommendations

1. **Immediate**: Document current encryption approach as "development only"
2. **Short-term**: Add audit logging for cloud account operations
3. **Medium-term**: Integrate cloud KMS for key management (Phase 3)
4. **Long-term**: Add API key scopes for cloud operations