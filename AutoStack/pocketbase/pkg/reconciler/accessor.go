package reconciler

import "sync/atomic"

// Phase 3.9.1 — Package-level accessor for the active Reconciler.
//
// HTTP route handlers (registered in main.go OnBeforeServe) need to read
// metrics and trigger incident reconstruction from the running reconciler.
// The Reconciler instance is created in OnAfterBootstrap (which fires AFTER
// OnBeforeServe), so the routes cannot capture it via closure.
//
// Trade-off: a package-level singleton is acceptable here because the
// reconciler is process-scoped (single instance per AutoStack process) and
// the alternative (passing it through every route) would invasively change
// many controller signatures.
//
// HONESTY: this is a singleton. If a future caller wants to run multiple
// reconcilers in the same process (e.g., for test parallelism), this
// pattern blocks them. Acceptable for the single-pod production model.
//
// Read/write are atomic.Pointer so handler reads do not race with
// reconciler construction at startup. Nil reads return a sentinel
// "not ready" response upstream rather than panicking.

var activeReconciler atomic.Pointer[Reconciler]

// SetActive stores the process-wide reconciler reference. Called once at
// startup from main.go after Reconciler.Start(). Idempotent: subsequent
// calls replace the reference (useful for tests that recreate the
// reconciler; never expected in production).
func SetActive(r *Reconciler) {
	activeReconciler.Store(r)
}

// Active returns the current process reconciler or nil if not started.
// Handlers MUST nil-check the return.
func Active() *Reconciler {
	return activeReconciler.Load()
}
