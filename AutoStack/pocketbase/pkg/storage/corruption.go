package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/events"
)

// CorruptionType classifies what kind of integrity violation was detected.
type CorruptionType string

const (
	CorruptionTypeInvalidHash         CorruptionType = "invalid_hash"
	CorruptionTypeTruncatedPayload    CorruptionType = "truncated_payload"
	CorruptionTypeClockRegression     CorruptionType = "clock_regression"
	CorruptionTypeCheckpointMismatch  CorruptionType = "checkpoint_mismatch"
	CorruptionTypeOrphanedStream      CorruptionType = "orphaned_stream"
	CorruptionTypeReplayDivergence    CorruptionType = "replay_divergence"
	CorruptionTypeWALEntryHashInvalid CorruptionType = "wal_entry_hash_invalid"
)

// CorruptionFinding is an immutable record of one detected integrity violation.
// Findings are NEVER silently discarded — they become incidents and quarantine state.
type CorruptionFinding struct {
	FindingID  string         `json:"finding_id"`
	Type       CorruptionType `json:"type"`
	AffectedID string         `json:"affected_id"` // eventID, streamID, WAL sequence, etc.
	Detail     string         `json:"detail"`
	DetectedAt string         `json:"detected_at"`
}

// NewCorruptionFinding creates an immutable CorruptionFinding with the current timestamp.
func NewCorruptionFinding(findingID string, t CorruptionType, affectedID, detail string) CorruptionFinding {
	return CorruptionFinding{
		FindingID:  findingID,
		Type:       t,
		AffectedID: affectedID,
		Detail:     detail,
		DetectedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// CorruptionReport is a tamper-evident set of corruption findings for one tenant.
// ReportHash covers all stable fields — GeneratedAt excluded (content-identity).
type CorruptionReport struct {
	ReportID    string              `json:"report_id"`
	TenantID    string              `json:"tenant_id"`
	Findings    []CorruptionFinding `json:"findings"`
	Severity    string              `json:"severity"`    // "none" | "warning" | "critical"
	GeneratedAt string              `json:"generated_at"`
	ReportHash  string              `json:"report_hash"`
}

// BuildCorruptionReport creates a CorruptionReport and computes its hash.
func BuildCorruptionReport(reportID, tenantID string, findings []CorruptionFinding) CorruptionReport {
	cp := make([]CorruptionFinding, len(findings))
	copy(cp, findings)
	severity := computeSeverity(cp)
	r := CorruptionReport{
		ReportID:    reportID,
		TenantID:    tenantID,
		Findings:    cp,
		Severity:    severity,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.ReportHash = computeReportHash(r)
	return r
}

// VerifyCorruptionReport recomputes the hash and compares it to the stored value.
func VerifyCorruptionReport(r CorruptionReport) (bool, string) {
	stored := r.ReportHash
	r.ReportHash = ""
	expected := computeReportHash(r)
	if stored != expected {
		return false, fmt.Sprintf("report hash mismatch: stored=%s computed=%s", stored, expected)
	}
	return true, ""
}

// CorruptionFindingAsEvent converts a finding to a PlatformEvent so it enters the audit trail.
// The event is observation-only — it records the finding, not a remediation.
func CorruptionFindingAsEvent(finding CorruptionFinding, tenantID, eventID string, clock int64) events.PlatformEvent {
	payload, _ := json.Marshal(map[string]any{
		"finding_id":  finding.FindingID,
		"type":        string(finding.Type),
		"affected_id": finding.AffectedID,
		"detail":      finding.Detail,
	})
	return events.NewPlatformEvent(
		eventID,
		events.EventTypeContradictionDetected,
		events.AggregateTypeVerification,
		finding.AffectedID,
		"",
		tenantID,
		clock,
		"", "",
		payload,
		nil,
	)
}

// CorruptionFindingsFromIntegrityReport converts storage integrity violations
// into CorruptionFindings.
func CorruptionFindingsFromIntegrityReport(report StorageIntegrityReport) []CorruptionFinding {
	var findings []CorruptionFinding
	for i, eventID := range report.InvalidHashes {
		findings = append(findings, NewCorruptionFinding(
			fmt.Sprintf("hash-%s-%d", report.StreamID, i),
			CorruptionTypeInvalidHash,
			eventID,
			fmt.Sprintf("event %q in stream %q has invalid hash", eventID, report.StreamID),
		))
	}
	for i, cr := range report.ClockRegressions {
		findings = append(findings, NewCorruptionFinding(
			fmt.Sprintf("clock-%s-%d", report.StreamID, i),
			CorruptionTypeClockRegression,
			cr.EventID,
			fmt.Sprintf("event %q clock %d regresses from %d in stream %q",
				cr.EventID, cr.Clock, cr.PreviousClock, report.StreamID),
		))
	}
	return findings
}

// CorruptionFindingsFromWAL converts invalid WAL entries into CorruptionFindings.
func CorruptionFindingsFromWAL(badSequences []int64) []CorruptionFinding {
	findings := make([]CorruptionFinding, 0, len(badSequences))
	for _, seq := range badSequences {
		findings = append(findings, NewCorruptionFinding(
			fmt.Sprintf("wal-seq-%d", seq),
			CorruptionTypeWALEntryHashInvalid,
			fmt.Sprintf("wal:seq:%d", seq),
			fmt.Sprintf("WAL entry at sequence %d has invalid hash", seq),
		))
	}
	return findings
}

func computeSeverity(findings []CorruptionFinding) string {
	if len(findings) == 0 {
		return "none"
	}
	for _, f := range findings {
		switch f.Type {
		case CorruptionTypeInvalidHash, CorruptionTypeClockRegression,
			CorruptionTypeWALEntryHashInvalid, CorruptionTypeReplayDivergence:
			return "critical"
		}
	}
	return "warning"
}

func computeReportHash(r CorruptionReport) string {
	h := sha256.New()
	fmt.Fprintf(h, "report:%s|tenant:%s|severity:%s|findings:", r.ReportID, r.TenantID, r.Severity)
	for _, f := range r.Findings {
		fmt.Fprintf(h, "%s/%s/%s,", f.FindingID, f.Type, f.AffectedID)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
