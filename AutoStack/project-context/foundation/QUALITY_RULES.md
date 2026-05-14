# QUALITY_RULES.md — Engineering Quality Standards for AutoStack

> **This file defines what "good code" means in this repository.** It is enforced through code review, automated checks, and AI agent operating rules. Quality is not subjective. There are specific, measurable standards. This file lists them.

---

## Core Quality Principles

The principles below are non-negotiable. Every other quality rule derives from them.

### Principle 1: Reliability Over Cleverness

A function that is verbose but obvious beats a function that is short but tricky. Infrastructure code is read many more times than it is written. Optimize for the reader.

### Principle 2: Explicit Over Implicit

The code states what it does. Hidden behavior is forbidden. Magic numbers, magic strings, implicit type coercion, surprising defaults — all are forbidden.

### Principle 3: Fail Loudly

Errors are surfaced, not swallowed. Unexpected conditions trigger logs, alerts, or both. Silent failure is the worst failure mode in infrastructure software.

### Principle 4: Test What Matters

Tests cover behaviors, not implementations. A passing test does not mean correct code; it means the test does not detect the problem. Tests must actually exercise the scenarios users care about.

### Principle 5: Compose, Don't Inherit

Inheritance hierarchies create coupling. Composition through interfaces creates clarity. Go does not have inheritance for a reason; embrace it.

### Principle 6: Boundaries Are Sacred

Layers do not leak into each other. Frontend does not have business logic. Database access does not happen in HTTP handlers. Cloud-specific code lives in cloud provider packages. See `SYSTEM_BOUNDARIES.md`.

---

## Code Style Rules

### Go Code Style

#### File Organization
- One package per directory.
- Package name matches directory name (lowercase, no underscores).
- File names use snake_case (e.g., `cloud_account.go`, `audit_log.go`).
- Each file should have a clear, singular purpose.
- File length should rarely exceed 500 lines. If it does, the package likely needs to be split.
- File starts with the package declaration. No blank lines before it.

#### Naming
- Exported names: `PascalCase`.
- Unexported names: `camelCase`.
- Package names: `lowercase` (e.g., `providers`, not `Providers`).
- Constants: `PascalCase` if exported, `camelCase` if not. Avoid `SCREAMING_SNAKE_CASE` in Go.
- Acronyms in names follow Go convention: `URL` not `Url`, `ID` not `Id`, `API` not `Api`.
- Boolean variables and functions: `is...`, `has...`, `can...`, `should...`.

Examples of bad names:
- `data` (what data?)
- `helper` (helper for what?)
- `manager` (manages what?)
- `Process` (process what?)
- `temp` (this will outlive your patience)

Examples of good names:
- `cloudAccount`
- `validateCredentials`
- `deploymentNotFound`
- `isReconciliationDue`

#### Functions
- Functions should do one thing. The function name should describe what it does.
- Function length should rarely exceed 50 lines. If it does, the function likely needs to be split.
- Functions with more than 4 parameters are a smell. Use a struct.
- Return values are explicit. If you return an error, name the error variable in the return signature for clarity.
- Functions that mutate their arguments must say so in their documentation.
- Functions that have side effects (writes to disk, network calls) must say so in their documentation.

```go
// GOOD: clear, single-purpose function.
// EstimateMonthlyCost calculates the expected monthly cost in USD for the given deployment.
// Returns an error if the deployment's provider is unknown or the pricing API call fails.
func EstimateMonthlyCost(ctx context.Context, deployment *Deployment) (decimal.Decimal, error) {
    // implementation
}

// BAD: vague, does too much, hidden side effects.
func Process(d *Deployment) (decimal.Decimal, error) {
    // calculates cost, also updates the deployment's last-estimated timestamp, also sends a notification
}
```

#### Error Handling
- Every error is handled. The pattern `_ = someFunc()` to ignore errors is forbidden unless explicitly justified with a comment explaining why.
- Errors are wrapped with context using `fmt.Errorf("...: %w", err)`.
- Errors at API boundaries are converted to structured error responses (see `API_CONTRACTS.md`).
- Sentinel errors (`errors.New("not found")`) are exported from the package and checked with `errors.Is`.
- Custom error types should implement the `error` interface and provide structured fields for context.

```go
// GOOD: error is wrapped with context.
account, err := repo.GetCloudAccount(ctx, accountID)
if err != nil {
    return fmt.Errorf("loading cloud account %s: %w", accountID, err)
}

// BAD: error context is lost.
account, err := repo.GetCloudAccount(ctx, accountID)
if err != nil {
    return err
}

// BAD: error is silently ignored.
account, _ := repo.GetCloudAccount(ctx, accountID)
```

#### Context Usage
- Functions that perform I/O or can be cancelled take `context.Context` as the first parameter.
- The context name is `ctx`, not `context` or `c`.
- Contexts are propagated to all downstream calls. Never create a `context.Background()` mid-call chain.
- Contexts carry request metadata (request ID, user ID, tenant ID) via typed keys, never string keys.
- Goroutines spawned for background work get a context derived from a long-lived service context, not from a request context (which will be cancelled when the request ends).

#### Comments
- Exported declarations require a godoc comment. Format: `// FunctionName does X...`.
- Comments explain *why*, not *what*. The code already shows what.
- Comments are kept up to date. Stale comments are worse than no comments.
- Long comments use `/* */` style for paragraphs, `//` for single-line.
- TODO comments include the author and a reference: `// TODO(raj): refactor when blueprint versioning ships (ISSUE-010)`.
- FIXME comments are stronger than TODO and require a tracked issue.

#### Goroutines
- Every goroutine has a clear shutdown path. Goroutines that block forever are forbidden.
- Goroutines that wait on channels select with a context for cancellation.
- Long-running goroutines (background services) log when they start and stop, including reason for stopping.
- Goroutines that can crash recover gracefully and log the panic. Use a `defer recover()` at the goroutine entry point.

```go
// GOOD: goroutine has clear shutdown via context.
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Error("reconciler panic", "panic", r, "stack", string(debug.Stack()))
        }
    }()

    ticker := time.NewTicker(s.config.ReconcileInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            log.Info("reconciler shutting down")
            return
        case <-ticker.C:
            s.reconcileOnce(ctx)
        }
    }
}()

// BAD: no shutdown path, no panic recovery, no logging.
go func() {
    for {
        s.reconcileOnce()
        time.Sleep(30 * time.Second)
    }
}()
```

#### Imports
- Imports are grouped: standard library, third-party, internal.
- Within each group, imports are alphabetical.
- `gofmt` and `goimports` are run on every save. CI verifies.
- No unused imports.
- No dot imports except for `embed` directives in tests where allowed.
- Internal package imports use the full module path, not relative.

#### Concurrency Primitives
- Prefer channels for communication.
- Use `sync.Mutex` only when channels are awkward.
- `sync.RWMutex` only when you have profiled and confirmed a contention issue with `sync.Mutex`.
- `sync.Map` is rarely the right answer. Profile first.
- `atomic` operations are for counters and flags, not for orchestrating control flow.
- Goroutine fan-out uses `errgroup.Group` for cancellation and error collection.

#### Resource Management
- Every `Open`, `Get`, or `Acquire` is paired with a `defer Close`, `defer Release`, etc.
- HTTP response bodies are always closed.
- Database rows and transactions are always closed/committed/rolled back.
- File handles are always closed.
- Goroutines holding resources must release them before exiting.

```go
// GOOD: deferred cleanup.
resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()

// BAD: forgotten close.
resp, err := client.Do(req)
if err != nil {
    return err
}
body, _ := io.ReadAll(resp.Body)
// resp.Body is never closed
```

---

### Frontend / TypeScript Style

#### File Organization
- One component per file.
- File names use kebab-case (e.g., `deployment-card.svelte`).
- Co-locate component files, their styles, and their tests.
- Shared utilities go in `frontend/src/lib/`.

#### TypeScript Strictness
- `tsconfig.json` has `"strict": true`.
- No `any` types except where unavoidable (and document why).
- Prefer `unknown` over `any` when the type is genuinely unknown.
- Use discriminated unions for state machines (e.g., `{ status: 'loading' } | { status: 'success', data: T } | { status: 'error', error: E }`).

#### Component Rules
- Components are small. A component over 200 lines should probably be split.
- Components have a single responsibility.
- Components take props with explicit types.
- Events bubble up via Svelte's `createEventDispatcher`, not via prop callbacks.
- State that is only used by the component lives in the component. State shared across components lives in a Svelte store.

#### Reactive Statements
- Reactive statements (`$:` in Svelte) are sparingly used. Over-reactivity creates infinite loops.
- Side effects in reactive statements are clearly marked with a comment.
- Async operations in reactive statements are debounced or guarded against duplicate calls.

#### Styling
- Tailwind classes are used directly in markup for one-off styles.
- Common patterns are extracted into reusable components.
- Custom CSS only for what Tailwind cannot do.
- No inline styles except for dynamically computed values (e.g., width based on percentage).

#### State Management
- Server state is loaded via the API and cached using Svelte stores.
- Form state is local to the form component.
- Global UI state (theme, sidebar collapsed, etc.) lives in a `uiStore`.
- WebSocket message updates flow into Svelte stores, which trigger reactive UI updates.

#### API Calls
- All API calls go through `frontend/src/lib/api/`.
- API call functions return typed promises.
- API errors are handled at the call site.
- No business logic in API layer; just HTTP and parsing.

#### Routing
- SvelteKit's file-based routing is the source of truth.
- Route guards (auth checks) happen in `+layout.server.ts` or `+page.server.ts` where applicable.
- Public routes are explicit; the default is authenticated.

#### Imports
- Imports grouped: third-party, `$lib/...`, relative.
- No deep relative imports (`../../../`). Use `$lib/` alias.
- No unused imports.

---

## Testing Standards

### Coverage Expectations
- Provider implementations: 80% line coverage minimum.
- Reconciler logic: 80% line coverage minimum.
- Cost calculation: 90% line coverage minimum.
- HTTP handlers: 70% line coverage minimum.
- Frontend components: smoke tests + behavior tests for interactive components.

These percentages are *floors*, not goals. Coverage is necessary but not sufficient.

### Test Structure
- Tests use `_test.go` suffix.
- Test files live in the same directory as the code they test.
- Test function names: `TestSubjectMethod_Scenario` (e.g., `TestProvider_Deploy_ReturnsErrorWhenCredentialsInvalid`).
- Use table-driven tests for multiple variants.
- Each test sets up its own fixtures. No shared mutable state between tests.

```go
// GOOD: table-driven test.
func TestParseImageRef(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    ImageRef
        wantErr bool
    }{
        {
            name:  "registry with tag",
            input: "docker.io/library/nginx:1.27",
            want:  ImageRef{Registry: "docker.io", Repository: "library/nginx", Tag: "1.27"},
        },
        // ...more cases
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseImageRef(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("ParseImageRef() error = %v, wantErr %v", err, tt.wantErr)
            }
            if !tt.wantErr && got != tt.want {
                t.Errorf("ParseImageRef() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Mocking
- Mock external dependencies (cloud APIs, database, HTTP services) at the interface boundary.
- Generate mocks with `mockgen` from interfaces, do not hand-write mocks.
- Mock at the highest sensible abstraction level. Mocking `*http.Client` is finer-grained than mocking a `RegistryClient` interface.
- Real database (PocketBase in test mode) is used for integration tests; mocks for unit tests.

### Integration Tests
- Tagged with `//go:build integration` to be excluded from default test runs.
- Run against real PocketBase, real Kubernetes (via `kind` or `envtest`), real cloud provider sandboxes (where credentials are available).
- Live in `_integration_test.go` files or `tests/integration/` directory.
- Run in CI on a separate job from unit tests.

### E2E Tests
- Test the full stack: frontend → API → reconciler → cloud provider.
- Use Playwright for frontend automation.
- Use a dedicated test cloud account (never against production).
- Cleanup after every test (delete all created resources).
- Tagged with `e2e` group in package.json.

### What Not To Test
- Trivial getters and setters.
- Generated code.
- Third-party libraries (we trust them to test themselves).
- Configuration values (we test the code that uses them).

### What Must Be Tested
- Error paths. Every error returning code path is tested for the error case.
- Boundary conditions. Zero values, empty strings, nil slices, very large inputs.
- Concurrent code. Race conditions are tested with `go test -race`.
- Security-sensitive code. Credential redaction, authentication, authorization.
- Data migrations. Every migration is tested against the prior schema state.

---

## Documentation Standards

### What Must Be Documented
- Every exported Go function, type, and constant.
- Every PocketBase collection schema (in `DATA_MODEL.md`).
- Every API endpoint (in `API_CONTRACTS.md`).
- Every WebSocket message type (in `API_CONTRACTS.md`).
- Every architectural decision (in `DECISIONS.md`).
- Every known issue (in `KNOWN_ISSUES.md`).
- Every cloud provider's deployment flow (in provider-specific docs).
- Every environment variable (in `DEPLOYMENT_MODEL.md`).

### What Documentation Looks Like
- Explanation before code, not code before explanation.
- Concrete examples, not abstract patterns.
- Failure modes are documented, not just happy paths.
- Cross-references to related docs.
- Updated when the code changes. A stale doc is worse than no doc.

### Code Comments vs Markdown Documentation
- Code comments: how this specific code works, why it does what it does.
- Markdown documentation: what the system does, how layers interact, what decisions were made.
- Don't duplicate. If something is in the markdown docs, the code comment can reference it.

---

## Logging Standards

### Log Levels
- `DEBUG`: development diagnostics, off by default in production
- `INFO`: normal operational events (service start, periodic status, user actions)
- `WARN`: unexpected but recoverable conditions
- `ERROR`: failures that require attention
- `FATAL`: unrecoverable, service is about to exit

### Structured Logging
- All logs use structured fields, not string formatting.
- Common fields: `request_id`, `user_id`, `tenant_id`, `deployment_id`, `provider`.
- Field names use snake_case.
- Field values are typed (no `"true"` strings, use `true` booleans).

```go
// GOOD: structured logging.
log.Info("deployment created",
    "deployment_id", deployment.ID,
    "project_id", deployment.ProjectID,
    "target_type", deployment.TargetType,
    "duration_ms", duration.Milliseconds(),
)

// BAD: unstructured.
log.Printf("created deployment %s in project %s of type %s in %dms", deployment.ID, deployment.ProjectID, deployment.TargetType, duration.Milliseconds())
```

### What Not To Log
- Credentials, passwords, API keys, tokens.
- Full request/response bodies (may contain credentials).
- Personally identifying information (PII).
- Secrets values, even in error contexts.

### Redaction
- When a credential reference is needed in a log, log the redacted form: first 3 + `***` + last 2 characters.
- When a secret reference is needed, log the secret ID, not the value.

---

## Security Standards

### Secrets Handling
- Credentials are stored encrypted (see `SECURITY_AND_ACCESS.md`).
- Credentials are loaded into memory only when needed.
- Credentials are cleared from memory after use where possible.
- Credentials are never serialized to logs, traces, or metrics.

### Input Validation
- All API inputs are validated against a schema before being used.
- Validation happens at the API boundary, not deep in business logic.
- Validation rejects unknown fields (no silent ignoring).
- Validation returns structured error responses with specific field-level details.

### Authentication and Authorization
- Every protected endpoint checks authentication.
- Every endpoint that touches user data checks authorization for the specific resource.
- "Just check the user is logged in" is not authorization. Check the user has access to this specific resource.
- Authorization errors return 403, not 401 (401 is for missing/invalid credentials).
- Authorization errors do not leak information about whether the resource exists.

### SQL Injection / Injection Attacks
- No string concatenation in queries. Use parameterized queries always.
- No string concatenation in shell commands. Use `exec.Command` with separate arguments.
- No string concatenation in API URLs. Use `url.Values` or proper escaping.

### TLS Everywhere
- All external HTTP calls use TLS (HTTPS).
- TLS certificates are validated. No `InsecureSkipVerify: true` except in explicitly documented test code.
- Custom CA certificates are configurable for private registries; mechanism is documented and isolated.

### Cryptographic Operations
- Use the Go standard library where possible.
- Do not implement custom cryptography.
- Encryption uses AES-256-GCM (authenticated encryption).
- Hashing for non-security use cases: SHA-256.
- Hashing for password storage: bcrypt (handled by PocketBase).
- Random values for security: `crypto/rand`, not `math/rand`.

---

## Performance Standards

### Latency Budgets
- API endpoints: p95 under 500ms for read endpoints, under 1s for write endpoints.
- WebSocket message dispatch: p95 under 100ms from event to client.
- Background reconciliation cycle: completes within the configured interval.

### Resource Usage
- Backend memory: stays under 1 GB for typical workloads (100 deployments, 5 active users).
- Backend CPU: under 50% of one core for typical workloads.
- Database: stays under 80% of provisioned storage.
- Goroutine count: monitored, alert if it grows unboundedly.

### Performance Testing
- Load tests run weekly in CI against a staging environment.
- Profile any function that handles more than 100 requests per second.
- Profile any goroutine that loops continuously.
- Track metrics: request rate, error rate, latency percentiles, goroutine count, memory usage.

### Caching Rules
- Cache only when measured to help. Premature caching is a bug source.
- Every cache entry has a TTL.
- Every cache has size limits.
- Cache invalidation is deliberate, not accidental.
- Cache keys include tenant identifier in multi-tenant code paths.

---

## API Standards

(See `API_CONTRACTS.md` for full API documentation. Quality rules apply.)

### Versioning
- All API endpoints are under `/api/v1/`.
- Breaking changes go to a new version (`/api/v2/`).
- Deprecated endpoints emit a `Deprecation` header and a log warning.
- Deprecated endpoints are supported for at least 6 months.

### Error Responses
- Errors use the standard envelope (see ADR-028).
- Error codes are stable. Don't rename them once published.
- Error messages are human-readable.
- Error details provide structured information for client handling.

### Request and Response Sizes
- Request bodies are limited to a reasonable max (1 MB default, configurable).
- Response pagination is enforced. No unbounded list responses.

---

## Operational Standards

### Health Checks
- Every service exposes `/health` (liveness) and `/ready` (readiness).
- Liveness fails if the service is in an unrecoverable state.
- Readiness fails if the service is not ready to handle requests (DB unreachable, dependencies down).
- Health endpoints are unauthenticated.

### Metrics
- Every service exposes `/metrics` in Prometheus format.
- Standard metrics: request count, request latency, error count, goroutine count.
- Custom metrics: reconciliation cycle count, cost calculation latency, cloud API call count, etc.

### Tracing
- Distributed tracing via OpenTelemetry.
- Trace context propagated across service boundaries via standard headers.
- Sample rate configurable (default 1% in production, 100% in development).

### Configuration
- Configuration is loaded from environment variables.
- No configuration in code.
- Configuration values have defaults that work for local development.
- Production configuration is explicit, not relying on defaults.

### Graceful Shutdown
- Services receive SIGTERM, drain in-flight requests, then exit.
- Drain timeout is configurable (default 30 seconds).
- Background services stop their loops on context cancellation.
- Database connections are closed cleanly.

---

## Dependency Management

### Adding Dependencies
- Every new dependency requires an ADR entry.
- Evaluate: maintenance status, license, alternatives, security posture.
- Prefer well-maintained, widely-used libraries.
- Avoid libraries that bring large transitive dependency trees.

### Updating Dependencies
- Security updates: applied within 1 week of disclosure.
- Major version updates: planned, tested in CI, accompanied by ADR if behavior changes.
- Minor and patch updates: applied monthly via dependabot.

### Removing Dependencies
- If a dependency is no longer used, remove it.
- If a dependency is only used in a small area, evaluate whether to in-house that small piece of code.

---

## Code Review Standards

### What Reviewers Look For
- Does the code do what the PR description says?
- Are the tests adequate?
- Are there edge cases that are not handled?
- Is the code readable to someone unfamiliar with the area?
- Is the documentation updated?
- Are there security implications?
- Are there performance implications?
- Does it follow the conventions in this file?

### Review Comments
- Comments are constructive, not dismissive.
- Comments explain *why* a change is suggested.
- Comments distinguish between blocking (must fix) and non-blocking (nice to have).
- Comments cite documentation where applicable.

### PR Description Standards
Every PR description includes:
- What the change does
- Why the change is being made
- What was tested
- Any deployment considerations
- Related issues or ADRs

### Approval Standards
- PRs to critical paths (operator, security, cost calculation) require 2 approvals.
- PRs to other code require 1 approval.
- All PRs require passing CI before merge.

---

## Definition of Done

A change is "done" when:

1. The code compiles and passes lint.
2. All tests pass (unit, integration where relevant).
3. The code is reviewed and approved.
4. Documentation is updated.
5. No new lint warnings introduced.
6. No new security findings introduced.
7. ADRs added if architectural decisions were made.
8. KNOWN_ISSUES updated if issues were discovered.
9. The change is verified in a staging environment before production deploy.

Anything less is in progress. There is no "done except for X." X must be done.

---

## Code Smell Catalog

The following patterns are smells. Their presence is not always a bug, but is a signal to investigate.

### Functions Doing Too Much
A function whose name has "and" in it usually does two things. Split it.

### Nested Conditionals More Than 2 Deep
Refactor with early returns, guard clauses, or extracted functions.

### Comments Explaining What
If a comment says "this loop iterates over deployments and checks their status," the code is unclear. Improve the code, remove the comment.

### Boolean Parameters
Functions with boolean parameters often need two functions. `Deploy(force bool)` becomes `Deploy()` and `ForceDeploy()`.

### Long Parameter Lists
More than 4 parameters: pass a struct.

### God Objects
Types with many methods that handle many concerns. Split by responsibility.

### Stringly Typed Code
Functions taking strings where enums or typed constants would be safer. Define types.

### Magic Numbers
`if count > 5` should be `if count > MaxRetries`. Name the constant.

### Premature Optimization
Caching, indexing, batching done without profiling. Often makes code worse without making it faster.

### Premature Generalization
Building a flexible framework when one concrete case is needed. YAGNI applies.

---

## How To Handle Disagreement With These Rules

You may believe a rule is wrong in your specific case. The process:

1. Document the case in a PR description.
2. Propose an exception or rule change.
3. If accepted, the rule is updated in this file with the exception case documented.
4. If rejected, follow the rule.

You do not unilaterally violate a rule and hope it slips through review. Quality rules are a team agreement. Changes go through discussion.

---

## Final Note

These rules are not exhaustive. They cover the issues we have encountered or anticipated. As new issues emerge, this file will grow. Treat it as a living document.

The goal is not to enforce rules for their own sake. The goal is to ship software that we can maintain, that does not surprise us at 3am, that users can trust. Every rule in this file derives from a moment when its absence caused pain.
