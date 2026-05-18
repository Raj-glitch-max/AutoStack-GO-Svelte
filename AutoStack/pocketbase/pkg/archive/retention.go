package archive

import (
	"time"
)

// RetentionPolicy defines how long archive entries are kept.
// AutoStack archives are append-only — retention policies govern export
// and cleanup eligibility only, never actual deletion without forensic export first.
type RetentionPolicy struct {
	MinRetentionDays int  `json:"min_retention_days"`
	MaxRetentionDays int  `json:"max_retention_days"`
	ForensicRequired bool `json:"forensic_required"` // if true, a forensic export must precede cleanup
}

// DefaultRetentionPolicy is the production minimum: 365 days, forensic export required.
var DefaultRetentionPolicy = RetentionPolicy{
	MinRetentionDays: 365,
	MaxRetentionDays: 2555, // ~7 years
	ForensicRequired: true,
}

// RetentionEligibility describes whether an entry is eligible for cleanup.
type RetentionEligibility struct {
	EntryID          string `json:"entry_id"`
	Eligible         bool   `json:"eligible"`
	DaysOld          int    `json:"days_old"`
	ForensicExported bool   `json:"forensic_exported"`
	Reason           string `json:"reason,omitempty"`
}

// EvaluateRetention determines whether an archive entry is eligible for cleanup
// based on the retention policy and whether a forensic export exists.
// Eligibility does NOT mean deletion — it means the entry may be considered for cleanup.
func EvaluateRetention(entry ArchiveEntry, policy RetentionPolicy, forensicExported bool, now time.Time) RetentionEligibility {
	archivedAt, err := time.Parse(time.RFC3339Nano, entry.ArchivedAt)
	daysOld := 0
	if err == nil {
		daysOld = int(now.Sub(archivedAt).Hours() / 24)
	}

	if daysOld < policy.MinRetentionDays {
		return RetentionEligibility{
			EntryID: entry.EntryID,
			Eligible: false,
			DaysOld:  daysOld,
			Reason:   "minimum retention period not elapsed",
		}
	}

	if policy.ForensicRequired && !forensicExported {
		return RetentionEligibility{
			EntryID: entry.EntryID,
			Eligible: false,
			DaysOld:  daysOld,
			Reason:   "forensic export required before cleanup",
		}
	}

	return RetentionEligibility{
		EntryID:          entry.EntryID,
		Eligible:         true,
		DaysOld:          daysOld,
		ForensicExported: forensicExported,
	}
}

// FilterEligible returns only entries that are cleanup-eligible per the policy.
func FilterEligible(entries []ArchiveEntry, policy RetentionPolicy, forensicExported bool, now time.Time) []RetentionEligibility {
	var out []RetentionEligibility
	for _, e := range entries {
		eval := EvaluateRetention(e, policy, forensicExported, now)
		if eval.Eligible {
			out = append(out, eval)
		}
	}
	return out
}
