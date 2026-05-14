# Structured Logging Proposal — Phase 2.7

## Last Updated
2026-05-14

## Status

**Documented; not landing in Phase 2.7.** Deferred to Phase 2.9 / Phase 3.

## Premise

Every log emission today is `log.Printf` with `[TAG] field=value`
substring formatting. This works but:

- Field extraction requires brittle grep + regex.
- Alert systems must parse strings.
- Levels are not encoded (everything is at default level).
- No correlation across process boundaries.

`log/slog` (stdlib since Go 1.21) provides structured logging with
field-typed key/value pairs, levels, and JSON output. Migration is
mechanical but touches every call site.

## Proposed migration shape

```go
import "log/slog"

slog.InfoContext(ctx, "DISPATCH_CLAIM",
    "cycle", cycleID,
    "target", targetID,
    "operation", opID,
    "rollout_revision", rolloutRevision,
    "action", historyAction,
)
```

Levels:
- INFO: normal lifecycle events.
- WARN: suspicion holds, transition refused, cycle backed off.
- ERROR: panics, decrypt failures, hard error outcomes.
- DEBUG: heartbeat ticks, cycle starts.

JSON output to stderr by default; configurable via
`AUTOSTACK_LOG_FORMAT=json|text`.

## Why deferred to Phase 2.9 / 3

- Migration is mechanical but touches ~100+ call sites.
- Risk of accidental log-message rename breaking external alerts.
- Phase 2.7 priority is forensic-history fixes, not logging refactor.
- Phase 2.9 production-readiness gate is the appropriate point to
  decide whether slog adoption is required before exit.

## Bridge

Phase 2.7 ships informal "[TAG]" emissions with consistent
key=value pairs. A `log/slog`-like grammar is followed today
informally (cycle=X target=Y …). Future slog migration can preserve
the tag names as `slog.Logger.With(slog.String("tag", "DISPATCH_CLAIM"))`
or as the first positional message.

## What lands in Phase 2.7 without slog

- 5 forensic-completeness fixes documented in
  [[forensic-completeness-assessment]].
- Continued discipline on `[TAG] key=value` informal structure.

## Related
- [[../phase2.3/observability-integrity]]
- [[../phase2.4/incident-reconstruction-maturity-review]]
