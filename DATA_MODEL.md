# DATA_MODEL.md — AutoStack Data Model

---

## Source of Truth Rules

| Data Type | Source of Truth | Sync Direction |
|---|---|---|
| Deployment desired state | PocketBase `rollouts` | PocketBase → Kubernetes CRD / Cloud API |
| Kubernetes actual state | Kubernetes API | Kubernetes → PocketBase status fields |
| Cloud actual state | Cloud Provider API | Cloud API → PocketBase status fields via Reconciler |
| User identity | PocketBase `users` | Not synced |
| Cloud account credentials | PocketBase `cloud_accounts` (encrypted) | Not synced |
| Cost estimates | PocketBase `cost_estimates` | Regenerated on demand |
| Actual cost records | PocketBase `cost_records` | Cloud Billing API → PocketBase |
| Audit trail | PocketBase `audit_log` | Append-only, never modified |
| Rollout history | PocketBase `rollout_history` | Append-only, never modified |

**Conflict Resolution Rule**: PocketBase desired state always wins. If cloud actual state diverges from PocketBase desired state, the reconciler corrects the cloud state toward PocketBase. Never update PocketBase to match cloud without user intent.

---

## Collections: Protected (Existing Kubernetes System)

> **DO NOT ALTER SCHEMA. ANY CHANGE REQUIRES A MIGRATION FILE.**

### `users` (PocketBase Built-In Auth Collection)
```
id              string (auto)
email           string (unique)
username        string
verified        bool
avatar          file
display_name    string
created         datetime
updated         datetime
```
**Extension planned**: `organization_id` relation (not yet added)

### `projects`
```
id              string (auto)
name            string (required)
description     string
owner           relation → users
namespace       string (Kubernetes namespace name)
cluster_config  json (encrypted) {
    server_url:             string
    certificate_authority:  string (base64)
    service_account_token:  string (encrypted)
}
created         datetime
updated         datetime
```
**Extension planned**: `workspace_id` relation (not yet added, must not break existing records when added)

### `rollouts`
```
id              string (auto)
name            string (required)
description     string
project         relation → projects
spec            json (RolloutSpec — full deployment configuration)
status          string  enum: pending|deploying|running|failed|rolled_back|stopped
image_tag       string  (current deployed image tag)
replicas_available   int
last_deployed   datetime
created         datetime
updated         datetime
```
**Planned additions** (additive, nullable, with defaults):
```
target_type     string  enum: kubernetes|ecs|cloudrun|aca  default: "kubernetes"
target_config   json    (provider-specific config)          default: null
cloud_account   relation → cloud_accounts                   default: null (nullable)
endpoint_url    string  (public URL after deployment)        default: null
```

### `rollout_history`
```
id              string (auto)
rollout         relation → rollouts
spec_snapshot   json (complete RolloutSpec at this point in history)
changed_by      relation → users
change_type     string  enum: create|update|rollback|auto-update|cloud-migrate
previous_tag    string
new_tag         string
diff_summary    string (human-readable summary of what changed)
created         datetime
```
**IMMUTABLE. Never update or delete records in this collection.**

### `blueprints`
```
id              string (auto)
name            string (required)
description     string
owner           relation → users
spec            json (BlueprintSpec — deployment template)
shared_with     relation[] → users
category        string (web|worker|database|cache|queue|proxy|other)
is_public       bool
compatible_targets   string[] enum values: kubernetes|ecs|cloudrun|aca
version         string  (semver, e.g., "1.0.0")
created         datetime
updated         datetime
```

### `auto_update_policies`
```
id              string (auto)
rollout         relation → rollouts
policy_type     string  enum: semver|timestamp|disabled
poll_interval_seconds    int  (minimum: 60)
allowed_tag_pattern      string (regex)
update_schedule          string (cron expression, optional)
last_checked    datetime
last_updated_tag         string
enabled         bool
created         datetime
updated         datetime
```

### `registry_credentials`
```
id              string (auto)
name            string
owner           relation → users (or workspace when workspace model added)
registry_url    string (e.g., "registry-1.docker.io", "ghcr.io", "myorg.jfrog.io")
username        string
password_encrypted       string (AES-256, key from AUTOSTACK_ENCRYPTION_KEY env var)
last_validated  datetime
validation_status        string  enum: valid|invalid|unknown
created         datetime
updated         datetime
```

---

## Collections: New (Cloud Integration)

### `organizations`
```
id              string (auto)
name            string (required)
slug            string (unique, URL-safe)
owner           relation → users
billing_email   string
plan            string  enum: free|team|enterprise
sso_config      json (nullable) {
    provider:       string (okta|azure-ad|google-workspace|saml)
    sso_url:        string
    entity_id:      string
    certificate:    string
}
settings        json {
    default_region:         string
    allowed_providers:      string[]
    require_cost_approval:  bool
    cost_approval_threshold_usd: float
}
created         datetime
updated         datetime
```

### `workspaces`
```
id              string (auto)
name            string (required)
organization    relation → organizations
description     string
settings        json {
    max_deployments:        int (0 = unlimited)
    max_monthly_cost_usd:   float (0 = unlimited)
    allowed_regions:        string[]
    allowed_providers:      string[]
}
created         datetime
updated         datetime
```

### `workspace_members`
```
id              string (auto)
workspace       relation → workspaces
user            relation → users
role            string  enum: admin|developer|viewer|billing
invited_by      relation → users
invited_at      datetime
accepted_at     datetime
created         datetime
```

### `cloud_accounts`
```
id              string (auto)
name            string (display name for this account)
workspace       relation → workspaces
provider        string  enum: aws|gcp|azure
region          string  (primary region)
credentials_encrypted    json (AES-256 encrypted, structure varies by provider) {
    -- AWS --
    access_key_id:      string
    secret_access_key:  string
    role_arn:           string (optional, for role assumption)
    external_id:        string (optional, for cross-account role)

    -- GCP --
    service_account_json: string (full service account key JSON)
    project_id:           string

    -- Azure --
    tenant_id:          string
    client_id:          string
    client_secret:      string
    subscription_id:    string
}
status          string  enum: active|error|validating|revoked
validated_at    datetime
last_validated  datetime
validation_error         string (human-readable, null if valid)
created         datetime
updated         datetime
```

### `deployment_targets`
```
id              string (auto)
rollout         relation → rollouts
cloud_account   relation → cloud_accounts
provider        string  enum: aws-ecs|gcp-cloudrun|azure-aca
region          string
external_id     string (ECS service ARN, Cloud Run service name, ACA container app name)
status          string  enum: pending|creating|running|updating|stopped|error|deleting|deleted
endpoint_url    string (public URL once live, null until available)
last_synced     datetime (last time reconciler polled this target)
drift_detected  bool (true if actual state diverges from desired)
drift_summary   string (human-readable description of drift)
created         datetime
updated         datetime
```

### `network_configs`
```
id              string (auto)
cloud_account   relation → cloud_accounts
region          string
provider        string  enum: aws|gcp|azure
config          json {
    -- AWS --
    vpc_id:             string
    public_subnet_ids:  string[]
    private_subnet_ids: string[]
    security_group_ids: string[]
    autostack_managed:  bool (true if AutoStack created this VPC)

    -- GCP --
    network:            string
    subnetwork:         string
    autostack_managed:  bool

    -- Azure --
    resource_group:     string
    vnet_name:          string
    subnet_name:        string
    autostack_managed:  bool
}
created         datetime
updated         datetime
```

### `cost_estimates`
```
id              string (auto)
rollout         relation → rollouts
cloud_account   relation → cloud_accounts
cpu_vcpu        float
memory_gb       float
replicas        int
region          string

-- Compute costs (from pricing API) --
hourly_cpu_rate_usd      float
hourly_memory_rate_usd   float
compute_monthly_low_usd  float (minimum estimate)
compute_monthly_high_usd float (maximum estimate)

-- Infrastructure overhead (from Infracost API) --
lb_monthly_usd           float
nat_monthly_usd          float (null if not applicable)
logging_monthly_usd      float
other_monthly_usd        float

-- Summary --
total_monthly_low_usd    float
total_monthly_high_usd   float
uncertainty_note         string (explanation of what is not included)
pricing_source           string (e.g., "aws_pricing_api_v2_2024-01")
calculated_at            datetime
```

### `cost_records`
```
id              string (auto)
rollout         relation → rollouts
cloud_account   relation → cloud_accounts
date            date (daily record)
actual_cost_usd float
cost_breakdown  json {
    compute:        float
    data_transfer:  float
    storage:        float
    logging:        float
    load_balancer:  float
    other:          float
}
source          string (aws-cost-explorer|gcp-billing|azure-cost-management)
created         datetime
```

### `dns_records`
```
id              string (auto)
rollout         relation → rollouts
domain          string (e.g., "api.mycompany.com")
subdomain       string (e.g., "api")
target_url      string (the cloud endpoint to point to)
certificate_arn string (ACM ARN for AWS, etc.)
certificate_status       string  enum: pending|issued|failed|expired
dns_verified    bool
dns_record_type string (CNAME|A)
dns_value       string (the value to set in DNS)
verification_token       string (for domain ownership verification)
created         datetime
updated         datetime
```

### `audit_log`
> **APPEND-ONLY. NEVER UPDATE OR DELETE RECORDS.**
```
id              string (auto)
organization    relation → organizations (nullable for system-level events)
workspace       relation → workspaces (nullable)
user            relation → users (nullable for system-initiated actions)
api_key_id      string (nullable, if action was via API key)
action          string (structured action identifier, e.g., "rollout.create", "cloud_account.validate")
resource_type   string (rollout|project|cloud_account|blueprint|workspace|user|api_key)
resource_id     string
old_value       json (nullable, sanitized — no credentials)
new_value       json (nullable, sanitized — no credentials)
ip_address      string
user_agent      string
result          string  enum: success|failure|denied
error_message   string (nullable)
created         datetime
```

### `api_keys`
```
id              string (auto)
name            string
user            relation → users
workspace       relation → workspaces (scopes key to this workspace)
key_hash        string (bcrypt hash of the actual key — actual key shown only at creation)
key_prefix      string (first 8 chars for display identification, e.g., "ask_abc1")
scopes          string[] (e.g., ["rollout:read", "rollout:deploy", "rollout:delete"])
expires_at      datetime (nullable — null = no expiry)
last_used_at    datetime
created         datetime
revoked_at      datetime (nullable — null = not revoked)
```

### `notifications`
```
id              string (auto)
workspace       relation → workspaces
user            relation → users
channel         string  enum: email|slack|teams|webhook|in-app
trigger         string (e.g., "deploy.success", "deploy.failure", "cost.anomaly", "drift.detected")
destination     string (email address, slack webhook URL, teams webhook URL, generic URL)
enabled         bool
created         datetime
updated         datetime
```

### `incidents` (AI-assisted)
```
id              string (auto)
rollout         relation → rollouts
detected_at     datetime
resolved_at     datetime (nullable)
status          string  enum: open|investigating|resolved|false-positive
severity        string  enum: critical|high|medium|low
trigger         string (what triggered this incident record)
log_snapshot    text (the log lines at time of incident — truncated at 10,000 chars)
ai_explanation  json (nullable) {
    probable_cause:     string
    affected_component: string
    severity_assessment: string
    suggested_remediation: string[]
    confidence:         string (high|medium|low)
}
acknowledged_by  relation → users (nullable)
created         datetime
updated         datetime
```

---

## RolloutSpec Schema (The Deployment Configuration)

This JSON structure is stored in `rollouts.spec` and `rollout_history.spec_snapshot`. It is provider-agnostic.

```json
{
    "image": {
        "registry": "docker.io",
        "repository": "myorg/myapp",
        "tag": "1.2.3",
        "pull_policy": "IfNotPresent",
        "registry_credential_id": "optional-pocketbase-id"
    },
    "compute": {
        "cpu_request_vcpu": 0.25,
        "cpu_limit_vcpu": 0.5,
        "memory_request_mb": 256,
        "memory_limit_mb": 512
    },
    "scale": {
        "min_replicas": 1,
        "max_replicas": 3,
        "scale_metric": "cpu",
        "scale_target_percent": 70
    },
    "network": {
        "interfaces": [
            {
                "container_port": 8080,
                "protocol": "TCP",
                "service_type": "LoadBalancer",
                "ingress_host": "api.mycompany.com",
                "tls_enabled": true
            }
        ]
    },
    "env": [
        {"name": "LOG_LEVEL", "value": "info"},
        {"name": "APP_ENV", "value": "production"}
    ],
    "secrets": [
        {
            "name": "DATABASE_URL",
            "source": "kubernetes-secret",
            "secret_name": "db-credentials",
            "secret_key": "connection-string"
        }
    ],
    "volumes": [
        {
            "name": "uploads",
            "mount_path": "/app/uploads",
            "size_gb": 10,
            "storage_class": "standard"
        }
    ],
    "health": {
        "liveness_path": "/health",
        "liveness_port": 8080,
        "readiness_path": "/ready",
        "readiness_port": 8080,
        "initial_delay_seconds": 15,
        "period_seconds": 10,
        "failure_threshold": 3
    },
    "provider_overrides": {
        "cloudrun": {
            "min_instances": 0,
            "cpu_throttling": false,
            "execution_environment": "gen2"
        },
        "ecs": {
            "launch_type": "FARGATE",
            "assign_public_ip": false
        }
    }
}
```

---

## Data Integrity Rules

1. **Audit log is append-only**: No UPDATE or DELETE operations on `audit_log`. If a row is wrong, add a correcting row.
2. **Rollout history is append-only**: No UPDATE or DELETE on `rollout_history`. History is immutable.
3. **Credentials never in plaintext**: `credentials_encrypted` and `password_encrypted` fields are always AES-256 encrypted at rest. Never stored as plaintext. Never logged. Never returned in API responses (only the status/validation of the credential is returned, never the value).
4. **Deletion is soft for deployments**: Deleting a rollout sets `status = "deleting"`. The destroy operation is queued. Only after cloud resources are confirmed deleted does the record move to `status = "deleted"`. The record is never hard-deleted.
5. **Cost records are never modified**: Once written for a given day, a cost record should not be updated. If a cost recalculation is needed, add a new record and deprecate the old.
6. **Workspace-scoped cloud accounts**: Cloud account credentials are scoped to workspaces, never to individual users. A user's access to a cloud account is governed by their workspace membership role.

---

## Retention Policy

| Collection | Retention | Reason |
|---|---|---|
| `audit_log` | Forever (or 7 years for compliance) | SOC2, regulatory |
| `rollout_history` | Forever | User needs full history |
| `cost_records` | 3 years | Financial auditing |
| `incidents` | 2 years | Operational learning |
| `rollouts` (deleted status) | 90 days | Grace period for recovery |
| `cost_estimates` | 30 days | Recalculated on demand |
| `dns_records` (deleted) | 90 days | Grace period |
| All other collections | Until explicitly deleted | Standard |
