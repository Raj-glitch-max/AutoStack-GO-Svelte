package governance

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// NewAuditChain creates an empty, initialised audit chain for an execution.
func NewAuditChain(executionID string) GovernanceAuditChain {
	return GovernanceAuditChain{
		ExecutionID: executionID,
		ChainHash:   computeChainHash(nil),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}

// AppendAuditEntry appends a new entry to the chain and returns the updated chain.
// The new entry's PrevHash is set to the last entry's EntryHash (or "" if first).
// EntryHash is computed over the entry content + PrevHash.
//
// The chain is append-only: no entry is ever removed or modified.
// Returns the updated GovernanceAuditChain (value semantics — original is unchanged).
func AppendAuditEntry(chain GovernanceAuditChain, entry GovernanceAuditEntry) GovernanceAuditChain {
	// Set linking fields.
	if len(chain.Entries) > 0 {
		entry.PrevHash = chain.Entries[len(chain.Entries)-1].EntryHash
	} else {
		entry.PrevHash = ""
	}
	entry.EntryHash = computeEntryHash(entry)

	// Build updated chain.
	newEntries := append([]GovernanceAuditEntry(nil), chain.Entries...)
	newEntries = append(newEntries, entry)

	return GovernanceAuditChain{
		ExecutionID: chain.ExecutionID,
		Entries:     newEntries,
		ChainHash:   computeChainHash(newEntries),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}

// VerifyAuditChainIntegrity checks that every entry's EntryHash is correct
// and that the PrevHash chain is unbroken.
// Returns (true, "") if the chain is intact, or (false, reason) if tampered.
func VerifyAuditChainIntegrity(chain GovernanceAuditChain) (bool, string) {
	prevHash := ""
	for i, entry := range chain.Entries {
		if entry.PrevHash != prevHash {
			return false, fmt.Sprintf("entry[%d] PrevHash mismatch: expected %q got %q", i, prevHash, entry.PrevHash)
		}
		expected := computeEntryHash(entry)
		if entry.EntryHash != expected {
			return false, fmt.Sprintf("entry[%d] EntryHash tampered: expected %q got %q", i, expected, entry.EntryHash)
		}
		prevHash = entry.EntryHash
	}
	// Verify overall chain hash.
	expected := computeChainHash(chain.Entries)
	if chain.ChainHash != expected {
		return false, fmt.Sprintf("ChainHash mismatch: expected %q got %q", expected, chain.ChainHash)
	}
	return true, ""
}

// NewAuditEntry creates a new GovernanceAuditEntry with a stable ID and timestamp.
// EntryHash is set by AppendAuditEntry — callers should not set it manually.
func NewAuditEntry(entryID, executionID, actorID string, action AuditAction, policyID, roleID, reason string) GovernanceAuditEntry {
	return GovernanceAuditEntry{
		EntryID:     entryID,
		ExecutionID: executionID,
		ActorID:     actorID,
		Action:      action,
		PolicyID:    policyID,
		RoleID:      roleID,
		Reason:      reason,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// computeEntryHash derives SHA-256 over the entry content fields.
// PrevHash is included so the hash captures the chain position.
// EntryHash itself is excluded (it's the output).
func computeEntryHash(e GovernanceAuditEntry) string {
	raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s",
		e.EntryID, e.ExecutionID, e.ActorID, string(e.Action),
		e.PolicyID, e.RoleID, e.Reason, e.Timestamp, e.PrevHash,
	)
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum)
}

// computeChainHash derives SHA-256 over all entry hashes concatenated in order.
func computeChainHash(entries []GovernanceAuditEntry) string {
	if len(entries) == 0 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte("")))
	}
	combined := ""
	for _, e := range entries {
		combined += e.EntryHash
	}
	sum := sha256.Sum256([]byte(combined))
	return fmt.Sprintf("%x", sum)
}
