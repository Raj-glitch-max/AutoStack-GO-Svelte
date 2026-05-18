package accounting

import (
	"testing"
)

func makeRecord(t *testing.T, id string, rt ResourceType, execID, tenantID string, units int64) UsageRecord {
	t.Helper()
	return NewUsageRecord(id, rt, execID, tenantID, units, "events")
}

func TestNewUsageRecord_Fields(t *testing.T) {
	r := NewUsageRecord("r1", ResourceTypeExecution, "exec-1", "tenant-1", 42, "ms")
	if r.RecordID != "r1" || r.Units != 42 || r.UnitLabel != "ms" {
		t.Fatal("UsageRecord fields must match inputs")
	}
	if r.RecordedAt == "" {
		t.Fatal("RecordedAt must be set")
	}
}

func TestSummarizeUsage_FiltersByTenant(t *testing.T) {
	r1 := makeRecord(t, "r1", ResourceTypeExecution, "exec-1", "tenant-1", 10)
	r2 := makeRecord(t, "r2", ResourceTypeExecution, "exec-1", "tenant-2", 20)
	s := SummarizeUsage("tenant-1", "exec-1", []UsageRecord{r1, r2})
	if len(s.Records) != 1 {
		t.Fatalf("summary must only include tenant-1 records, got %d", len(s.Records))
	}
}

func TestSummarizeUsage_FiltersByExecution(t *testing.T) {
	r1 := makeRecord(t, "r1", ResourceTypeExecution, "exec-1", "tenant-1", 10)
	r2 := makeRecord(t, "r2", ResourceTypeExecution, "exec-2", "tenant-1", 20)
	s := SummarizeUsage("tenant-1", "exec-1", []UsageRecord{r1, r2})
	if len(s.Records) != 1 || s.Records[0].ExecutionID != "exec-1" {
		t.Fatal("summary must only include exec-1 records")
	}
}

func TestSummarizeUsage_TotalByType(t *testing.T) {
	r1 := makeRecord(t, "r1", ResourceTypeEvent, "exec-1", "tenant-1", 5)
	r2 := makeRecord(t, "r2", ResourceTypeEvent, "exec-1", "tenant-1", 3)
	s := SummarizeUsage("tenant-1", "exec-1", []UsageRecord{r1, r2})
	if s.TotalByType[ResourceTypeEvent] != 8 {
		t.Fatalf("total for ResourceTypeEvent must be 8, got %d", s.TotalByType[ResourceTypeEvent])
	}
}

func TestSummarizeUsage_SummaryHashNonEmpty(t *testing.T) {
	s := SummarizeUsage("tenant-1", "exec-1", nil)
	if s.SummaryHash == "" {
		t.Fatal("SummaryHash must be non-empty")
	}
}

func TestSummarizeUsage_Deterministic(t *testing.T) {
	r1 := makeRecord(t, "r1", ResourceTypeEvent, "exec-1", "tenant-1", 5)
	s1 := SummarizeUsage("tenant-1", "exec-1", []UsageRecord{r1})
	s2 := SummarizeUsage("tenant-1", "exec-1", []UsageRecord{r1})
	if s1.SummaryHash != s2.SummaryHash {
		t.Fatal("SummarizeUsage must be deterministic")
	}
}

func TestSummarizeUsage_AllExecutions(t *testing.T) {
	r1 := makeRecord(t, "r1", ResourceTypeEvent, "exec-1", "tenant-1", 5)
	r2 := makeRecord(t, "r2", ResourceTypeEvent, "exec-2", "tenant-1", 3)
	// Empty executionID means all executions for tenant.
	s := SummarizeUsage("tenant-1", "", []UsageRecord{r1, r2})
	if len(s.Records) != 2 {
		t.Fatalf("empty executionID must include all tenant records, got %d", len(s.Records))
	}
}

func TestRecordsForTenant_Correct(t *testing.T) {
	r1 := makeRecord(t, "r1", ResourceTypeEvent, "exec-1", "tenant-1", 1)
	r2 := makeRecord(t, "r2", ResourceTypeEvent, "exec-1", "tenant-2", 1)
	result := RecordsForTenant([]UsageRecord{r1, r2}, "tenant-1")
	if len(result) != 1 || result[0].TenantID != "tenant-1" {
		t.Fatal("RecordsForTenant must filter by tenant")
	}
}

func TestRecordsForExecution_Correct(t *testing.T) {
	r1 := makeRecord(t, "r1", ResourceTypeEvent, "exec-1", "tenant-1", 1)
	r2 := makeRecord(t, "r2", ResourceTypeEvent, "exec-2", "tenant-1", 1)
	result := RecordsForExecution([]UsageRecord{r1, r2}, "exec-1")
	if len(result) != 1 || result[0].ExecutionID != "exec-1" {
		t.Fatal("RecordsForExecution must filter by executionID")
	}
}

func TestTotalUnits_Sum(t *testing.T) {
	r1 := makeRecord(t, "r1", ResourceTypeEvent, "exec-1", "tenant-1", 7)
	r2 := makeRecord(t, "r2", ResourceTypeEvent, "exec-1", "tenant-1", 3)
	r3 := makeRecord(t, "r3", ResourceTypeArchival, "exec-1", "tenant-1", 100)
	total := TotalUnits([]UsageRecord{r1, r2, r3}, ResourceTypeEvent)
	if total != 10 {
		t.Fatalf("total events must be 10, got %d", total)
	}
}

func TestTotalUnits_NoMatch(t *testing.T) {
	r1 := makeRecord(t, "r1", ResourceTypeEvent, "exec-1", "tenant-1", 5)
	total := TotalUnits([]UsageRecord{r1}, ResourceTypeArchival)
	if total != 0 {
		t.Fatalf("unmatched resource type must return 0, got %d", total)
	}
}

func TestSummarizeUsage_MultipleResourceTypes(t *testing.T) {
	r1 := makeRecord(t, "r1", ResourceTypeEvent, "exec-1", "tenant-1", 10)
	r2 := makeRecord(t, "r2", ResourceTypeReplay, "exec-1", "tenant-1", 3)
	r3 := makeRecord(t, "r3", ResourceTypeVerification, "exec-1", "tenant-1", 2)
	s := SummarizeUsage("tenant-1", "exec-1", []UsageRecord{r1, r2, r3})
	if len(s.TotalByType) != 3 {
		t.Fatalf("must have 3 resource type totals, got %d", len(s.TotalByType))
	}
	if s.TotalByType[ResourceTypeEvent] != 10 {
		t.Fatal("event total must be 10")
	}
}
