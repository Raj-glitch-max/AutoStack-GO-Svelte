package audit

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ForensicRef is a typed reference to a forensic evidence item.
type ForensicRef struct {
	Kind string `json:"kind"` // journal, event, transition, approval, contradiction, observation, decision, certification
	ID   string `json:"id"`
	Hash string `json:"hash,omitempty"`
}

// ExecutionForensicsBundle is a tamper-evident collection of all forensic evidence
// for a single execution. BundledAt is excluded from BundleHash (content-identity).
type ExecutionForensicsBundle struct {
	BundleID     string        `json:"bundle_id"`
	ExecutionID  string        `json:"execution_id"`
	TenantID     string        `json:"tenant_id"`
	Journals     []ForensicRef `json:"journals"`
	Events       []ForensicRef `json:"events"`
	Transitions  []ForensicRef `json:"transitions"`
	Approvals    []ForensicRef `json:"approvals"`
	Contradictions []ForensicRef `json:"contradictions"`
	Observations []ForensicRef `json:"observations"`
	Decisions    []ForensicRef `json:"decisions"`
	Certifications []ForensicRef `json:"certifications"`
	BundledAt    string        `json:"bundled_at"` // excluded from hash
	BundleHash   string        `json:"bundle_hash"`
}

// BuildForensicsBundle assembles a tamper-evident forensics bundle.
func BuildForensicsBundle(
	bundleID, executionID, tenantID string,
	journals, events, transitions, approvals, contradictions, observations, decisions, certifications []ForensicRef,
) ExecutionForensicsBundle {
	b := ExecutionForensicsBundle{
		BundleID:       bundleID,
		ExecutionID:    executionID,
		TenantID:       tenantID,
		Journals:       copyRefs(journals),
		Events:         copyRefs(events),
		Transitions:    copyRefs(transitions),
		Approvals:      copyRefs(approvals),
		Contradictions: copyRefs(contradictions),
		Observations:   copyRefs(observations),
		Decisions:      copyRefs(decisions),
		Certifications: copyRefs(certifications),
		BundledAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	b.BundleHash = computeBundleHash(b)
	return b
}

// VerifyBundleHash recomputes the hash and returns (true, "") when it matches.
func VerifyBundleHash(b ExecutionForensicsBundle) (bool, string) {
	stored := b.BundleHash
	b.BundleHash = ""
	expected := computeBundleHash(b)
	if stored != expected {
		return false, fmt.Sprintf("bundle %q hash mismatch: stored=%s computed=%s",
			b.BundleID, stored, expected)
	}
	return true, ""
}

func copyRefs(refs []ForensicRef) []ForensicRef {
	out := make([]ForensicRef, len(refs))
	copy(out, refs)
	return out
}

func computeBundleHash(b ExecutionForensicsBundle) string {
	h := sha256.New()
	fmt.Fprintf(h, "bundle:%s|exec:%s|tenant:%s|journals:%d|events:%d|transitions:%d|approvals:%d|contradictions:%d|observations:%d|decisions:%d|certifications:%d",
		b.BundleID, b.ExecutionID, b.TenantID,
		len(b.Journals), len(b.Events), len(b.Transitions),
		len(b.Approvals), len(b.Contradictions), len(b.Observations),
		len(b.Decisions), len(b.Certifications))
	for _, r := range b.Journals {
		fmt.Fprintf(h, "|j:%s/%s", r.ID, r.Hash)
	}
	for _, r := range b.Events {
		fmt.Fprintf(h, "|e:%s/%s", r.ID, r.Hash)
	}
	for _, r := range b.Certifications {
		fmt.Fprintf(h, "|cert:%s/%s", r.ID, r.Hash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
