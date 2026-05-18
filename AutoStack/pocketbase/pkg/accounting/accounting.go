package accounting

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"
)

// NewUsageRecord creates an immutable usage record.
func NewUsageRecord(
	recordID string,
	resourceType ResourceType,
	executionID, tenantID string,
	units int64,
	unitLabel string,
) UsageRecord {
	return UsageRecord{
		RecordID:     recordID,
		ResourceType: resourceType,
		ExecutionID:  executionID,
		TenantID:     tenantID,
		Units:        units,
		UnitLabel:    unitLabel,
		RecordedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// SummarizeUsage aggregates all records for a tenant+execution pair.
// Records from other tenants or executions are excluded.
// TotalByType is computed deterministically (sorted resource type keys).
func SummarizeUsage(tenantID, executionID string, records []UsageRecord) UsageSummary {
	var filtered []UsageRecord
	for _, r := range records {
		if r.TenantID == tenantID && (executionID == "" || r.ExecutionID == executionID) {
			filtered = append(filtered, r)
		}
	}

	totals := make(map[ResourceType]int64)
	for _, r := range filtered {
		totals[r.ResourceType] += r.Units
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	s := UsageSummary{
		TenantID:    tenantID,
		ExecutionID: executionID,
		Records:     append([]UsageRecord(nil), filtered...),
		TotalByType: totals,
		GeneratedAt: now,
	}
	s.SummaryHash = computeSummaryHash(s)
	return s
}

// RecordsForTenant returns records matching a tenant, sorted by RecordedAt.
func RecordsForTenant(records []UsageRecord, tenantID string) []UsageRecord {
	var result []UsageRecord
	for _, r := range records {
		if r.TenantID == tenantID {
			result = append(result, r)
		}
	}
	sortRecords(result)
	return result
}

// RecordsForExecution returns records matching an executionID, sorted by RecordedAt.
func RecordsForExecution(records []UsageRecord, executionID string) []UsageRecord {
	var result []UsageRecord
	for _, r := range records {
		if r.ExecutionID == executionID {
			result = append(result, r)
		}
	}
	sortRecords(result)
	return result
}

// TotalUnits returns the sum of Units across all records of the given resource type.
func TotalUnits(records []UsageRecord, resourceType ResourceType) int64 {
	var total int64
	for _, r := range records {
		if r.ResourceType == resourceType {
			total += r.Units
		}
	}
	return total
}

func sortRecords(records []UsageRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].RecordedAt != records[j].RecordedAt {
			return records[i].RecordedAt < records[j].RecordedAt
		}
		return records[i].RecordID < records[j].RecordID
	})
}

func computeSummaryHash(s UsageSummary) string {
	h := sha256.New()
	fmt.Fprintf(h, "tenant:%s|exec:%s|", s.TenantID, s.ExecutionID)
	// Sort resource types for determinism.
	types := make([]string, 0, len(s.TotalByType))
	for rt := range s.TotalByType {
		types = append(types, string(rt))
	}
	sort.Strings(types)
	for _, rt := range types {
		fmt.Fprintf(h, "total:%s=%d|", rt, s.TotalByType[ResourceType(rt)])
	}
	for _, r := range s.Records {
		fmt.Fprintf(h, "record:%s:%d|", r.RecordID, r.Units)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
