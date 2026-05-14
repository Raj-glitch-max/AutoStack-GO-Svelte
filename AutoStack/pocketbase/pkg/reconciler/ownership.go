package reconciler

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"sync"
	"time"
)

// Phase 3.8 — SC-5 Ownership Fencing Foundation.
//
// This file establishes deterministic pod identity for the reconciler so
// every operation it claims is stamped with `owned_by_pod`. The identity
// is:
//
//   - generated ONCE at process start
//   - persisted in-memory only (a restart issues a new identity)
//   - written to operations.owned_by_pod on createOperation
//   - read by the sweep to log foreign-pod reclamations
//
// HONESTY NOTE: this does NOT make multi-pod deployment safe. Full
// multi-pod safety requires:
//
//   - lease-based coordination (Raft, etcd, or DB-backed lease)
//   - sweep that defers to live peer pods (heartbeat-aware peer
//     discovery rather than heartbeat-age alone)
//   - claimTarget CAS that excludes operations owned by live peer pods
//
// None of those are implemented. The pod identity stamp gives FORENSIC
// visibility (operators can see which pod did what) without any
// coordination guarantee.
//
// Anyone reading owned_by_pod must understand: it is a witness, not a
// fence. Running two reconciler pods will still produce double-dispatch.

// PodIdentity is a per-process identity string.
//
// Format: pod-<8-char hex>-<process-start-unix-seconds>
// Example: pod-3f2a91e0-1715300013
//
// 8 hex chars (32 bits) gives ~4 billion possible values. With expected
// deployment count <100 pods per cluster, collision is statistically zero.
// The Unix-seconds suffix makes restart history human-readable when
// inspecting operations.
type PodIdentity string

// generatePodIdentity creates a fresh identity at Reconciler.Start().
// Falls back to a deterministic fingerprint of hostname+pid if rand
// somehow fails — never returns empty.
func generatePodIdentity() PodIdentity {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		// Fallback: hostname-derived. Reasonable for forensic context.
		hostname, _ := os.Hostname()
		log.Printf("[POD_IDENTITY_RANDOM_ERR] falling back to hostname-derived id err=%v", err)
		// Hash via simple FNV-like (no crypto needed, this is a fallback)
		h := uint32(0)
		for i := 0; i < len(hostname); i++ {
			h = (h*16777619 + uint32(hostname[i])) & 0xFFFFFFFF
		}
		return PodIdentity(fmtPodID(uint32(h), time.Now().Unix()))
	}
	id := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	return PodIdentity(fmtPodID(id, time.Now().Unix()))
}

func fmtPodID(prefix uint32, ts int64) string {
	hex8 := hex.EncodeToString([]byte{byte(prefix >> 24), byte(prefix >> 16), byte(prefix >> 8), byte(prefix)})
	return "pod-" + hex8 + "-" + fmtUnix(ts)
}

func fmtUnix(ts int64) string {
	// Compact decimal representation; no leading zeros, no separators.
	const digits = "0123456789"
	if ts <= 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for ts > 0 {
		i--
		buf[i] = digits[ts%10]
		ts /= 10
	}
	return string(buf[i:])
}

// podIdentityHolder stores the current process's identity exactly once.
// Read-only after Start() completes.
var (
	podIdentity   PodIdentity
	podIdentityMu sync.RWMutex
)

// initPodIdentity is called once from Reconciler.Start() before any
// reconcile tick fires. Idempotent: subsequent calls log + return.
func initPodIdentity() PodIdentity {
	podIdentityMu.Lock()
	defer podIdentityMu.Unlock()
	if podIdentity != "" {
		return podIdentity
	}
	podIdentity = generatePodIdentity()
	log.Printf("[POD_IDENTITY] this_pod=%s", podIdentity)
	return podIdentity
}

// currentPodIdentity returns the identity for stamping operations.
// Safe for concurrent callers. Returns empty string if Start() hasn't run.
func currentPodIdentity() PodIdentity {
	podIdentityMu.RLock()
	defer podIdentityMu.RUnlock()
	return podIdentity
}
