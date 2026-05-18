# AutoStack Quickstart

**Target**: New developer operational in under 15 minutes.

---

## Prerequisites

- Docker >= 20.10
- Docker Compose v2
- 2 GB RAM, 10 GB free disk

No cloud credentials required for local setup.

---

## Step 1: Install (2 minutes)

```bash
# From the repo root:
bash AutoStack/pocketbase/scripts/install/install.sh

# Or with a custom install location:
AUTOSTACK_DIR=/opt/autostack bash AutoStack/pocketbase/scripts/install/install.sh
```

When the installer finishes, you'll see:
```
Platform UI:   http://localhost:8090/_/
API:           http://localhost:8090/api/v1/
```

---

## Step 2: Create Admin Account (1 minute)

1. Open http://localhost:8090/_/
2. Create your admin account (email + password)
3. Sign in

---

## Step 3: Deploy Your First Container (5 minutes)

Navigate to **Projects → New Project**:

1. Enter a project name (e.g., `my-app`)
2. Navigate to **Deployments → New Deployment**
3. Enter your container image (e.g., `nginx:latest`)
4. Configure port (e.g., `80`)
5. Click **Deploy**

The deployment will provision to your connected Kubernetes cluster (if configured) or show cloud provider options.

---

## Step 4: Watch It Deploy (real-time)

- **Logs**: Deployments → [your deployment] → Logs
- **Events**: Deployments → [your deployment] → Events
- **Status**: The status badge updates in real-time via WebSocket

---

## Step 5: Explore the Platform (5 minutes)

| Surface | What to look at |
|---|---|
| Platform → Overview | Active executions, health status |
| Platform → Timeline | Execution phases, governance decisions |
| Platform → Forensics | Hash verification, replay manifests |
| Platform → Governance | Policy decisions, approval status |
| Platform → Drills | Run simulated failure scenarios |

---

## Connecting a Kubernetes Cluster

AutoStack ships with Kubernetes support ready. To connect:

1. Ensure your `kubectl` context points to your cluster
2. AutoStack auto-discovers the cluster via `KUBECONFIG` (in-cluster when deployed in Kubernetes)
3. Projects deploy via the `one-click.dev/v1alpha1 Rollout` CRD

See KUBERNETES_EXISTING_SYSTEM.md for the full operator setup.

---

## Connecting a Cloud Provider (AWS, GCP, Azure)

1. Platform → Providers → New Account
2. Select provider and enter credentials
3. Credentials are encrypted with AES-256-GCM (see docs/security-boundaries.md)
4. Validate the account — AutoStack checks that the credentials can reach the provider API

Once connected, deployments targeting that provider will appear in the reconciler.

---

## Stopping AutoStack

```bash
docker compose -f ~/.autostack/docker-compose.yaml down
```

Data persists in `~/.autostack/data/` — restart and everything is where you left it.

---

## Troubleshooting

**PocketBase won't start**: Check `docker compose logs pocketbase`. Common cause: port 8090 already in use.

**"Encryption key not configured"**: Set `AUTOSTACK_ENCRYPTION_KEY` in `~/.autostack/.env` and restart.

**No Kubernetes cluster found**: AutoStack runs without a cluster — cloud provider deployments work independently.

**Drills fail**: Drill mode is simulation-only. If a drill fails, the simulation itself detected a problem — see the drill result for operator action.

---

## What to Read Next

- `docs/runtime-architecture.md` — How the platform works
- `docs/governance-model.md` — How approvals and policies work
- `docs/security-boundaries.md` — What the platform secures (and what it doesn't)
- `docs/distributed-boundaries.md` — Single-node constraints
