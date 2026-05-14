# SECURITY_AND_ACCESS.md — AutoStack Security and Access Model

---

## Security Philosophy

Security in AutoStack operates on four principles:

1. **Credentials never leave their encrypted home.** Credentials stored in PocketBase are AES-256 encrypted. They are decrypted only in-memory at the moment of use. They are never returned in API responses. They never appear in logs at any log level. They never appear in error messages.

2. **The platform is never more trusted than necessary.** AutoStack requests only the minimum cloud permissions needed to manage containers. It does not request billing modification rights. It does not request database administration rights. It does not request IAM administration rights. Every required permission is documented and justified.

3. **Every action is attributable.** Every state change — deployment, rollback, credential rotation, user access grant — is recorded in the immutable audit log with who, what, when, and from where. There is no silent operation.

4. **Running workloads are isolated from the platform.** Customer workloads running on their Kubernetes clusters or cloud accounts are not in the same security context as AutoStack. A breach of AutoStack does not automatically breach customer workloads. Credentials are encrypted at rest. The attack surface is the management plane, not the workload data plane.

---

## Authentication

### Session Authentication (Web UI)
- **Method**: PocketBase JWT-based sessions
- **Token lifetime**: 7 days (configurable)
- **Refresh**: Automatic refresh on activity, forced re-auth after expiry
- **Storage**: Frontend stores JWT in memory (not localStorage — see CLAUDE.md). Persists via httpOnly cookie.
- **MFA**: TOTP-based MFA for all accounts (planned — not yet implemented; must be implemented before enterprise launch)
- **Social Login**: GitHub, Google, Microsoft via OAuth2 (currently working)
- **SSO (Enterprise)**: SAML 2.0 and OIDC via WorkOS or equivalent — planned

### API Key Authentication (Programmatic Access / CI-CD)
- **Generation**: Users create named API keys from the settings UI or API
- **Storage**: Only the bcrypt hash is stored in PocketBase. The plaintext key is shown exactly once at creation. If lost, user must revoke and create a new key.
- **Format**: `ask_` prefix + 32 random bytes (hex-encoded). Example: `ask_a3f2...`
- **Scope**: Every key has explicit permission scopes. A key cannot perform actions outside its declared scopes.
- **Expiry**: Optional expiry date. Platform sends notification 7 days before expiry.
- **Transmission**: API keys sent in `Authorization: Bearer ask_...` header. Never in query parameters (query params appear in access logs).
- **Workspace Scope**: API keys are scoped to a specific workspace. A key from Workspace A cannot access Workspace B resources.

### Service-to-Service (Internal)
- Backend to PocketBase: embedded Go SDK (same process, no network auth)
- Cloud Reconciler to Backend: internal function calls (same process)
- Background services: no separate auth — same process, not network-accessible

---

## Authorization (RBAC)

### Organization-Level Roles

| Role | Description |
|---|---|
| `org:owner` | Full control. Can delete organization, manage billing, configure SSO, assign org admins. |
| `org:admin` | Can manage workspaces, invite users, configure SSO. Cannot delete organization. |
| `org:member` | Has workspace memberships. No org-level management capabilities. |
| `org:billing` | Read-only access to billing data across all workspaces. No deployment access. |

### Workspace-Level Roles

| Role | Capabilities |
|---|---|
| `workspace:admin` | Full control within workspace. Manage cloud accounts, set quotas, invite members. |
| `workspace:developer` | Create/update/delete deployments, manage blueprints, view costs. Cannot manage cloud accounts or quotas. |
| `workspace:viewer` | Read-only. View deployments, logs, metrics, costs. Cannot deploy or modify. |
| `workspace:billing_viewer` | Read cost reports only. Cannot view deployment configs, logs, or secrets. |

### API Key Scopes

```
rollout:read            → GET any rollout in the workspace
rollout:deploy          → Create new rollouts, trigger deployments
rollout:update          → Update existing rollout configuration
rollout:delete          → Delete rollouts (initiates destroy workflow)
rollout:rollback        → Trigger rollbacks
blueprint:read          → List and view blueprints
blueprint:write         → Create and update blueprints
cloud_account:read      → View cloud account status (not credentials)
cost:read               → View cost estimates and records
logs:read               → Stream logs from deployments
admin:full              → All scopes (dangerous — reserved for automation admin use only)
```

### Permission Inheritance Rules
- Workspace role permissions are additive (a developer can do everything a viewer can)
- API key scopes are restrictive (a key can only do what its explicitly declared scopes allow)
- Org admin does NOT inherit workspace admin permissions by default — they must be explicitly added to each workspace
- Resource-level permissions: a developer in Workspace A cannot access Workspace B resources even if both are in the same organization
- `workspace:admin` cannot grant permissions they themselves do not have (no privilege escalation)

### Forbidden Access Scenarios
- A `workspace:developer` cannot view, modify, or delete cloud account credentials
- A `workspace:viewer` cannot access secret values in any deployment
- A user with only `logs:read` API scope cannot view deployment configuration
- Any user from a different workspace cannot see the existence of another workspace's resources
- The platform's own service processes cannot access secrets outside of the active deployment operation

---

## Secret Handling

### What Is a "Secret" in AutoStack Context

Secrets include:
- Cloud provider credentials (AWS access keys, GCP service account keys, Azure client secrets)
- Container registry credentials (Docker Hub passwords, ECR tokens)
- Deployment secrets (environment variables declared as sensitive)
- API keys (hashed, not the raw value)
- JWT signing keys
- Encryption keys

### Storage Rules

1. **Cloud credentials**: AES-256-GCM encrypted in PocketBase `credentials_encrypted` JSON field. Encryption key from `AUTOSTACK_ENCRYPTION_KEY` environment variable (minimum 32 bytes, generated at platform setup). The encryption key must be stored in a secret manager or environment variable system — never in a file committed to the repository.

2. **Registry credentials**: Same AES-256-GCM pattern in `registry_credentials.password_encrypted`.

3. **Deployment secrets**: For Kubernetes path — stored as Kubernetes Secrets in the cluster namespace. AutoStack stores only a reference (secret name + key name), never the value. For cloud path — stored in AWS Secrets Manager, GCP Secret Manager, or Azure Key Vault in the customer's own cloud account. AutoStack stores only a reference ID, never the value.

4. **Platform JWT secret**: Environment variable. Never in codebase.

5. **API keys**: bcrypt-hashed in PocketBase. Raw value shown once. Never stored in recoverable form after creation.

### Transmission Rules
- Credentials are decrypted in-memory only at the moment a cloud API call is made
- Credentials are never serialized to disk in decrypted form
- Credentials are never sent over the network outside of encrypted HTTPS to cloud provider APIs
- API responses never include credential values — only status (valid/invalid), name, and masked display (e.g., "AKIAIO...XXXXX")
- WebSocket messages never include credential values

### Logging Prohibition
The following field names, if they appear in any structured log entry, must be redacted to `[REDACTED]` by the logging middleware, regardless of log level:
```
password, secret, token, key, credential, api_key, access_key, secret_key,
private_key, client_secret, service_account, auth_token, bearer, authorization,
credentials, password_encrypted, credentials_encrypted
```

This is enforced at the logger middleware level — not per callsite. A developer must not need to remember to redact; the infrastructure does it automatically.

---

## Infrastructure Protection

### AutoStack Platform Deployment Security
- Platform must be deployed behind HTTPS. HTTP redirect to HTTPS is mandatory.
- PocketBase admin UI (`/_/`) must be disabled or firewall-protected in production. Platform admin access is via the AutoStack UI only.
- Database (SQLite file or PostgreSQL) must not be accessible from the internet.
- Environment variables containing secrets must use a secret manager (AWS Secrets Manager, Doppler, Infisical) — not plaintext files.
- CORS must be configured to allow only the platform's own frontend domain.

### Kubernetes Operator Security
- The operator does not run with `cluster-admin` in production deployments
- Minimum required RBAC (Helm chart enforces this):
  ```yaml
  Rules:
    - Rollouts CRD: get, list, watch, create, update, patch, delete
    - Deployments (apps/v1): get, list, watch, create, update, patch, delete
    - Services: get, list, watch, create, update, patch, delete
    - Ingresses (networking.k8s.io): get, list, watch, create, update, patch, delete
    - HorizontalPodAutoscalers (autoscaling): get, list, watch, create, update, patch, delete
    - PersistentVolumeClaims: get, list, watch, create, delete
    - Pods: get, list, watch (no create — Deployment manages pods)
    - Events: get, list, watch, create (for recording operator events)
    - Namespaces: get, list (for project namespace verification)
  ```
- The operator service account is namespace-scoped where possible, cluster-scoped only for cross-namespace CRD watching
- Operator image must be pulled from a verified registry. Image digest pinning in Helm chart.

### Cloud Account Permissions Model
AutoStack requires minimum permissions per provider. These must be documented, version-controlled, and provided as one-click policy installation instructions to users.

**AWS — Required IAM Permissions**:
```json
{
  "ECS": ["CreateService", "UpdateService", "DeleteService", "DescribeServices",
          "RegisterTaskDefinition", "DeregisterTaskDefinition", "DescribeTaskDefinition",
          "RunTask", "StopTask", "DescribeTasks", "ListTasks"],
  "EC2": ["DescribeVpcs", "DescribeSubnets", "DescribeSecurityGroups",
          "CreateSecurityGroup", "AuthorizeSecurityGroupIngress",
          "DescribeLoadBalancers"],
  "ElasticLoadBalancingV2": ["CreateLoadBalancer", "DeleteLoadBalancer",
          "CreateTargetGroup", "DeleteTargetGroup", "CreateListener", "DeleteListener",
          "RegisterTargets", "DeregisterTargets", "DescribeTargetHealth"],
  "CloudWatchLogs": ["CreateLogGroup", "CreateLogStream", "PutLogEvents",
          "DescribeLogGroups", "DescribeLogStreams", "GetLogEvents"],
  "ECR": ["GetAuthorizationToken", "BatchCheckLayerAvailability",
          "GetDownloadUrlForLayer", "BatchGetImage"]
}
```

**AWS — Explicitly NOT Required** (deny these to reduce blast radius):
- `iam:*` — no IAM management
- `s3:*` — no S3 access
- `rds:*` — no database management
- `ec2:TerminateInstances` — no instance termination

---

## Audit and Governance

### What Must Be Logged in Audit Log

Every write operation to the following resources:
- Create, update, delete on `rollouts`
- Create, delete on `cloud_accounts`
- Validate on `cloud_accounts`
- Create, revoke on `api_keys`
- Create, delete, modify on `workspace_members`
- Create, update, delete on `projects`
- Blueprint create, update, delete, share
- Any rollback operation
- Auto-update trigger (system action)
- Cost approval granted or denied
- DNS record creation or deletion
- Incident creation, acknowledgment, resolution

**Audit record must include**:
- User ID (or "system" for automated actions)
- API key ID if action was via API key
- Source IP address
- User-Agent string
- Exact timestamp (millisecond precision)
- Resource type and resource ID
- Action performed
- Result (success/failure/denied)
- Old and new values (sanitized — no credential values)

### Audit Log Access
- `workspace:admin` can read audit logs for their workspace
- `org:admin` can read audit logs across all workspaces in their organization
- No role can modify or delete audit log records
- Audit logs must be exportable (JSON, CSV) for compliance purposes

### Compliance Notes
- SOC2 Type II: audit log implementation, access controls, encryption at rest all contribute to evidence. Litestream backup enables point-in-time recovery.
- GDPR: user data deletion request requires deletion from `users` collection and redaction of user-identifying fields in `audit_log`. Audit log records themselves (the action/resource data) are retained — only the user identifier is redacted.
- HIPAA: if a customer deploys HIPAA workloads, AutoStack must not store PHI. Deployment configuration (env vars, etc.) must not contain PHI. Logs streamed through the platform must be handled by the customer's own log infrastructure.

---

## Dangerous Operation Governance

### Confirmation Required (User Must Explicitly Confirm)

| Operation | Required Confirmation |
|---|---|
| Delete a deployment | Type deployment name + confirm in modal |
| Delete a project (and all deployments) | Type project name + confirm in modal |
| Force-delete (bypass cloud cleanup) | Type "FORCE DELETE" + explicit warning shown |
| Revoke cloud account credentials | Confirm understanding that managed deployments will be affected |
| Delete organization | Org owner must confirm + 24-hour delay before execution |

### Dual Authorization (Two Different Users Must Approve) — Enterprise Feature

| Operation | Required for |
|---|---|
| Deploy to production workspace | When workspace setting `require_deploy_approval` is enabled |
| Cost approval (over threshold) | When workspace setting `require_cost_approval` is enabled and estimate exceeds threshold |
| Cloud account credential rotation | When workspace requires rotation approval |

### Operations That AI May Never Perform Autonomously
- Delete any cloud resource
- Modify or delete any PocketBase record in production
- Rotate credentials
- Modify RBAC or access permissions
- Execute a rollback without user confirmation
- Create cloud accounts or network configurations

---

## Incident Response Philosophy

### Security Incident Severity Levels

| Severity | Definition | Response Time |
|---|---|---|
| P0 — Critical | Credential exposure, data breach, unauthorized access to production | Immediate — under 1 hour |
| P1 — High | Auth bypass, privilege escalation, exposed secrets in logs | Same business day |
| P2 — Medium | Unauthorized read access, audit log gaps | Within 48 hours |
| P3 — Low | Configuration weaknesses, non-exploited vulnerabilities | Next sprint |

### Credential Compromise Response
1. Immediately revoke the compromised credential (in AutoStack + in the cloud provider directly)
2. Audit all operations performed with the compromised credential (audit log query)
3. Determine if any unauthorized cloud resources were created (orphan scan)
4. Notify affected workspace admins
5. Issue new credentials with updated permissions
6. Write incident report to `incidents` collection
7. Review how the credential was exposed — apply mitigations

### Breach Notification Policy
- Workspace admins are notified within 24 hours of a confirmed breach affecting their workspace
- Organization owners are notified of any breach
- Affected users are notified in compliance with applicable regulations (GDPR: within 72 hours to supervisory authority if high-risk)
