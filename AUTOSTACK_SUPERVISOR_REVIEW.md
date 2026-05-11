# AutoStack — Supervisor Review
**Reviewed by:** Claude (acting as tech lead / product supervisor)  
**Date:** May 11, 2026  
**Verdict:** Strong backend. Broken production image. No unified UX. Cannot ship as-is.

---

## Executive Summary

The backend is genuinely impressive — Universal Engine, AI features, multi-cloud credentials, Terraform executor, WebSocket streaming — all built. But the project has 5 hard blockers that make it unshippable right now, 3 things that will break the moment someone uses it, and a UX structure that directly contradicts the "one click" goal. Below is every finding, prioritized.

---

## 🔴 HARD BLOCKERS — Fix before anything else

### BLOCKER 1 — Production Dockerfile cannot run Terraform

The main `Dockerfile` builds on `gcr.io/distroless/static-debian12:nonroot`. This image has:
- No shell
- No package manager
- No Terraform binary
- No way to install anything

The Terraform executor in `pkg/terraform` calls the `terraform` binary as a subprocess. In production, this call will fail immediately with "executable file not found."

**What exists instead:** `Dockerfile.combined` installs Terraform — but uses version **1.6.6** (released 2023) while the rest of the project uses **1.9.5**. This is a separate problem.

**Fix:**
```dockerfile
# Use debian-slim as final stage, not distroless
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates curl unzip && rm -rf /var/lib/apt/lists/*

# Install Terraform 1.9.5 (match what CI validates against)
RUN curl -fsSL https://releases.hashicorp.com/terraform/1.9.5/terraform_1.9.5_linux_amd64.zip -o terraform.zip \
    && unzip terraform.zip \
    && mv terraform /usr/local/bin/ \
    && rm terraform.zip \
    && terraform --version

COPY --from=backend-builder /bin/autostack-server /autostack-server
COPY --from=frontend-builder /app/frontend/build /pb_public
COPY --from=backend-builder /app/pocketbase/pb_migrations /pb_migrations
COPY --from=backend-builder /app/pocketbase/templates /templates

VOLUME ["/pb_data"]
EXPOSE 8090
ENTRYPOINT ["/autostack-server"]
CMD ["serve", "--http=0.0.0.0:8090", "--publicDir=/pb_public"]
```

Delete `Dockerfile.combined`. Keep one Dockerfile. Period.

---

### BLOCKER 2 — Real API keys committed in `.env.example`

The `.env.example` file in the repo contains live, working API keys:

```
INFRACOST_API_KEY=ics_v1_0mi91GSQK8FpSEa4rzbJT9_EmqTYVtz3ohpHfMx8rlBnhEFXcwuoroAoygAheK0eWBEbNfGFwhat
RESEND_API_KEY=re_NARhuvHj_2X7xVcXz8DVmiW23vc38ZFZn
NVIDIA_API_KEY=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
```

The Gitleaks secret scan in `security.yml` runs with `fetch-depth: 0` (full history). It **will catch these immediately** and fail CI every single run. Worse — these keys are now in git history forever unless the history is rewritten.

**Fix — do this right now, before any other work:**
1. Revoke all 3 keys immediately from their respective dashboards (Infracost, Resend, NVIDIA)
2. Generate new keys
3. Replace `.env.example` with placeholder values only:
   ```
   INFRACOST_API_KEY=your_infracost_key_here
   RESEND_API_KEY=re_your_key_here
   NVIDIA_API_KEY=nvapi-your_key_here
   ```
4. Run: `git filter-repo --path .env.example --invert-paths` OR use BFG Repo Cleaner to purge from history
5. Force push the cleaned history
6. Add `.env.example` check to Gitleaks config to exclude it from secret scanning (since it's intentionally a template)

---

### BLOCKER 3 — CD workflow deploys to wrong Kubernetes deployment name

In `.github/workflows/cd.yml`:
```yaml
kubectl set image deployment/one-click \
  one-click=ghcr.io/...
```

The project is called **AutoStack**. There is almost certainly no Kubernetes deployment named `one-click` in the clusters. This means every push to `develop` or `main` runs `kubectl set image`, gets a "deployment not found" error, and the deploy job fails.

**Fix:** Check the actual deployment name in `deployment/` manifests and update both the staging and production steps to match. If the manifest is named `autostack`, the command should be:
```yaml
kubectl set image deployment/autostack \
  autostack=ghcr.io/${{ github.repository }}:sha-${{ needs.build-and-push.outputs.short-sha }} \
  -n autostack-staging
```

---

### BLOCKER 4 — Universal Engine has no frontend

The `pkg/universal` backend is fully built and tested. All 8 stages work. But there is **no frontend page** for it. The overview explicitly lists this as item #1 in the backlog:

> "Universal Engine frontend — Build the UI for POST /api/deployments/universal"

Right now a user cannot use the Universal Engine at all. They can only use the old blueprint-based flow — which defeats the entire "any app" purpose.

This is a blocker for the core value proposition.

---

### BLOCKER 5 — GCP and Azure cannot actually deploy

Credential management works. Templates are written and valid. But **there is no deployment controller** for GCP or Azure. Only AWS has `awsDeployments.go` with the full plan/review/apply/destroy flow.

GCP and Azure are credential stores pretending to be deployment systems. A user who enters GCP credentials, selects a GCP blueprint, and clicks "Deploy" gets nothing — there is no handler on the backend to run `terraform apply` for GCP.

The multi-cloud dashboard shows GCP and Azure as options but they cannot deploy. This is misleading and a bad user experience.

---

## 🟡 WILL BREAK IN USAGE — Fix before showing to anyone

### Issue 1 — Terraform validation CI is testing fake files, not real templates

In `ci.yml` and `terraform-validate.yml`, the "validate" steps create a minimal test `.tf` file with just a provider block, then validate it. They do NOT validate the actual blueprint files (`ecs-web-app.tf`, `full-stack.tf`, etc.).

This means broken templates will pass CI. The tests are green but prove nothing about the actual Terraform that gets executed when a user deploys.

**Fix:** Each validation step should `cd` into the actual template directory and run `terraform validate` there:
```yaml
- name: Validate ecs-web-app blueprint
  run: |
    cd pocketbase/templates/ecs-web-app
    terraform init -backend=false
    terraform validate
```
Each blueprint should be in its own subdirectory with its own `main.tf`.

---

### Issue 2 — Two conflicting Dockerfiles will confuse every agent and developer

`Dockerfile` = distroless, no Terraform, port 8090  
`Dockerfile.combined` = debian-slim, Terraform 1.6.6, port 8090  

Any agent working on the project will not know which one to modify. Any `docker build` command without `-f` uses `Dockerfile` — which cannot run Terraform. Delete `Dockerfile.combined` after fixing `Dockerfile` per Blocker 1.

---

### Issue 3 — AI Cost Optimizer recommendations are invisible

The weekly background job runs. It calls the NVIDIA API. It stores recommendations in the database. But there is **no UI component** that displays them. Users paying for deployments and accumulating optimization recommendations will never see them.

This is a fully built feature (backend) that is completely wasted (no frontend).

**Fix needed:** A "Savings available" card on the dashboard → recommendations list page. The data is already there.

---

## 🟠 UX PROBLEMS — Things that fight against "one click"

### Problem 1 — Three parallel deployment paths confuse users

Currently a user lands in the app and faces:
- Kubernetes deployment (working, polished)
- AWS blueprint selection (4 blueprints to choose from)
- Universal Engine (exists on backend, no UI yet)

These should be **one flow**, not three. The goal is one click. A user should not need to know what ECS Fargate is, or that they need to pick a blueprint.

**What the flow should be:**
```
User lands → "Deploy your app" → provides Git URL or Docker image
→ system analyzes it (Universal Engine)
→ shows: "Here's what we found: Node.js/Express API, needs PostgreSQL"
→ shows: "Estimated cost: $42/month on AWS | $38/month on GCP | $45/month on Azure"
→ user picks cloud + clicks Deploy
→ done
```

The blueprint selection UI should be **removed from sight** for new users. Keep it as an "Advanced" option for power users who want manual control.

---

### Problem 2 — Blueprint selection puts the burden on the user

Selecting a blueprint requires the user to understand:
- What ECS Fargate is
- Whether their app is "full-stack" or "serverless" or "static"
- What an ALB is
- The difference between RDS and running postgres in a container

This is the opposite of "one click." The Universal Engine exists precisely to remove this burden. Build its frontend and route all new users through it.

---

### Problem 3 — Cost estimate appears too late

Currently cost estimation is shown during the deployment setup flow, after the user has already made infrastructure decisions. By that point they're committed mentally.

The cross-cloud comparison (AWS vs GCP vs Azure cost side-by-side) is built but only accessible on the multi-cloud dashboard — not on the main deploy flow.

**Fix:** Show the cross-cloud cost comparison BEFORE the user selects a cloud. "Here's your app on 3 clouds. Pick one." This is the most powerful UX moment and it's currently buried.

---

### Problem 4 — "Apply fix and retry" button exists but the Auto-Fixer already runs

The AI Error Recovery system runs the auto-fix loop (up to 3 retries) during deployment. But the UI shows an "Apply fix and retry" button, implying the user needs to manually trigger something.

This creates confusion: did the AI already try to fix it? Did it fail? Should I click the button?

**Fix:** Be explicit in the UI. If the auto-fix loop ran and succeeded, say so. If all 3 retries failed, show what each retry tried and why it failed. If the user can take manual action, be specific about what that action is.

---

## ✅ WHAT IS GENUINELY WORKING AND GOOD

Don't touch these — they are solid:

| Component | Status | Notes |
|---|---|---|
| Kubernetes deployment | ✅ Solid | Real-time logs, rollout history, works |
| AWS Terraform executor | ✅ Solid | Isolated dirs, per-user queue, WebSocket streaming |
| AWS cost estimation (Infracost) | ✅ Solid | Pre-deploy, min/estimate/max, 1h cache |
| AWS actual cost tracking | ✅ Solid | Daily CE fetch, incremental, variance |
| Cost anomaly alerts | ✅ Solid | Configurable threshold, email, in-app, ack |
| AI Deployment Advisor | ✅ Works | NVIDIA API, structured JSON output |
| AI Anomaly Explainer | ✅ Works | Attached to every alert, purple badge in UI |
| AI Error Recovery | ✅ Works | Pattern + LLM, recovery dashboard |
| Universal Engine (backend) | ✅ All 22 tests pass | Needs frontend only |
| GCP/Azure credential storage | ✅ Works | AES-256-GCM, validate on save |
| Cross-cloud pricing APIs | ✅ Works | GCP Billing + Azure Retail Prices |
| Webhook system | ✅ Complete | HMAC, retry, full CRUD, test endpoint |
| Alert preferences | ✅ Complete | Per-user threshold, email toggle, frequency |
| CI pipeline structure | ✅ Good | 5 workflows, security scans, matrix jobs |
| Rate limiting | ✅ Done | On all sensitive endpoints |
| AES-256-GCM encryption | ✅ Done | Credentials never stored plaintext |

---

## 📋 EXACT TASK LIST FOR AGENTS (Priority Order)

### Immediate (this week, do not proceed without these)

**Task I-1** — Fix `.env.example`: revoke real keys, replace with placeholders, clean git history  
**Task I-2** — Fix `Dockerfile`: switch final stage from distroless to debian-slim, install Terraform 1.9.5, delete `Dockerfile.combined`  
**Task I-3** — Fix `cd.yml`: correct the K8s deployment name in both staging and production deploy steps  
**Task I-4** — Fix `terraform-validate.yml` and `ci.yml`: validate actual template files, not a dummy provider-only file  

### High Priority (blocks "any app" goal)

**Task H-1** — Build Universal Engine frontend (`/app/deploy/universal` route):
- Step 1: Input form — Git URL or Docker image or natural language (3 tabs)
- Step 2: Analysis result card — shows detected language, framework, required services
- Step 3: Cross-cloud cost comparison — AWS / GCP / Azure side-by-side (use existing API)
- Step 4: "Deploy to [chosen cloud]" confirmation button with 10-min timeout gate
- Wire to `POST /api/deployments/universal` and `POST /api/deployments/:id/confirm`
- Real-time log stream via WebSocket (same component as AWS deployment detail)

**Task H-2** — Build GCP deployment controller (`pocketbase/pkg/controller/gcpDeployments.go`):
- Mirror `awsDeployments.go` exactly
- Routes: plan, confirm, destroy, status
- Use existing `pkg/terraform` executor with GCP credentials exported as env vars
- `GOOGLE_CREDENTIALS` env var pointing to the decrypted service account JSON

**Task H-3** — Build Azure deployment controller (`pocketbase/pkg/controller/azureDeployments.go`):
- Mirror `awsDeployments.go`
- Routes: plan, confirm, destroy, status
- Export: `ARM_CLIENT_ID`, `ARM_CLIENT_SECRET`, `ARM_TENANT_ID`, `ARM_SUBSCRIPTION_ID`

**Task H-4** — Surface AI Cost Optimizer recommendations in UI:
- Add "💡 Savings available" card to project dashboard (show total potential saving)
- Build `/app/projects/:id/optimizer` page with recommendation list
- Each recommendation: resource name, current cost, potential saving, action button
- "Apply" button → trigger the Terraform variable change via existing executor

### Medium Priority (UX cleanup)

**Task M-1** — Refactor the main navigation and deploy entry point:
- Primary CTA: "Deploy your app" → goes to Universal Engine UI (Task H-1)
- Move blueprint selection to `Settings → Advanced → Manual blueprint`
- Remove blueprint cards from the main deploy flow

**Task M-2** — Fix Error Recovery UI messaging:
- Show which auto-fix attempt number ran (1/3, 2/3, 3/3)
- If all failed: show what each attempt tried and the specific error it got
- Only show "retry" button if the auto-fix loop is not currently running

**Task M-3** — Move cross-cloud cost comparison to the deploy flow:
- Show it at Step 3 of every deploy (Universal Engine and existing AWS flow)
- Currently it's only on the multi-cloud dashboard page
- This is the killer feature — make it visible where the user makes the decision

### Keep as-is (do not touch)

- `pkg/terraform` executor — working, do not modify
- `pkg/intelligence` NVIDIA API integration — working
- `pkg/crypto` encryption — working
- `pkg/aws` credential chain — working  
- K8s deployment flow — working, don't break it
- Webhook system — complete
- Alert system — complete
- `pkg/universal` backend — all 22 tests pass, leave it

---

## Decision: NVIDIA deepseek-v4-pro vs Claude API for Terraform Generation

The system currently uses NVIDIA's deepseek-v4-pro for all AI features including Terraform generation. This needs a real evaluation:

**Risk with deepseek for Terraform generation:** Code generation quality matters more here than conversational quality. A bad Terraform output that passes the auto-fix loop but has subtle logic errors (wrong security group rules, missing IAM policy, incorrect subnet routing) could deploy to production with security holes. The auto-fix loop only catches `terraform validate` errors — not logical security errors.

**Recommendation:** Keep NVIDIA/deepseek for the conversational AI features (advisor, explainer, optimizer). Use Claude API specifically for `terraform_generator.go` and `ai_analyzer.go` where correctness matters more than speed. You have the `CLAUDE_API_KEY` env var already defined. Use it for infrastructure generation. This is the one place where AI quality directly affects production security.

**Implementation:** In `terraform_generator.go`, change the `callAIAPI` call to use the Anthropic API (`claude-sonnet-4-20250514`). Keep all other `pkg/intelligence` functions using NVIDIA. This is one file change with high safety impact.

---

## One Final Thing

The project overview says "Current Status: All 4 phases complete. Production-ready." — **this is not accurate**. The project is 70% there. The backend architecture is excellent. But with the Dockerfile unable to run Terraform, real API keys in git, a CD workflow that references a nonexistent K8s deployment, and no UI for the Universal Engine, this cannot be called production-ready.

That's fixable. The hardest parts (the architecture, the AI integration, the multi-cloud credential management, the Terraform executor) are all done correctly. What remains is the last mile — plumbing the backend to the UI, fixing the deployment artifacts, and tightening the UX into a single coherent flow.

Fix the 5 blockers → build the Universal Engine frontend → surface the GCP/Azure controllers → and you have something genuinely worth shipping.
