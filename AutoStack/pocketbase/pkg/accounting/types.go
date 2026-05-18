// Package accounting implements Phase 5.0 resource accounting.
// Pure accounting only — no billing, no pricing, no cost projection.
//
// All records are append-only. Summaries are derived from records.
package accounting

// ResourceType identifies the kind of resource being tracked.
type ResourceType string

const (
	ResourceTypeExecution    ResourceType = "execution"
	ResourceTypeProviderAPI  ResourceType = "provider_api"
	ResourceTypeReplay       ResourceType = "replay"
	ResourceTypeVerification ResourceType = "verification"
	ResourceTypeArchival     ResourceType = "archival"
	ResourceTypeEvent        ResourceType = "event"
)

// UsageRecord is one immutable accounting entry.
type UsageRecord struct {
	RecordID     string       `json:"record_id"`
	ResourceType ResourceType `json:"resource_type"`
	ExecutionID  string       `json:"execution_id"`
	TenantID     string       `json:"tenant_id"`
	// Units is the measured quantity (bytes, call count, event count, milliseconds, etc.).
	Units     int64  `json:"units"`
	UnitLabel string `json:"unit_label"` // "bytes", "api_calls", "events", "ms"
	RecordedAt string `json:"recorded_at"`
}

// UsageSummary aggregates records for one tenant+execution pair.
type UsageSummary struct {
	TenantID    string                  `json:"tenant_id"`
	ExecutionID string                  `json:"execution_id"`
	Records     []UsageRecord           `json:"records"`
	TotalByType map[ResourceType]int64  `json:"total_by_type"`
	SummaryHash string                  `json:"summary_hash"`
	GeneratedAt string                  `json:"generated_at"`
}
