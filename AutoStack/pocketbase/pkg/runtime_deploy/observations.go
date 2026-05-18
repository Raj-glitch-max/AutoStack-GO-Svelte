package runtime_deploy

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/models"
)

// ObservationSource classifies WHO produced an observation.
type ObservationSource string

const (
	SourceEngine   ObservationSource = "engine"
	SourceOperator ObservationSource = "operator"
	SourceDocker   ObservationSource = "docker"
	SourceExternal ObservationSource = "external"
)

// ObservationTrust classifies the strength of an observation.
type ObservationTrust string

const (
	TrustDirect   ObservationTrust = "direct"   // result of a real subprocess call
	TrustInferred ObservationTrust = "inferred" // derived from a direct observation
	TrustAsserted ObservationTrust = "asserted" // operator-stated, not verified
)

// Observation is one append-only record about a deployment's observed state.
// Observations live in their own collection (runtime_observations) and do NOT
// mutate the deployment record; the reconciler reads observations and decides
// whether to transition state.
type Observation struct {
	ID           string            `json:"id"`
	DeploymentID string            `json:"deployment_id"`
	Source       ObservationSource `json:"source"`
	Trust        ObservationTrust  `json:"trust"`
	Confidence   float64           `json:"confidence"`
	Kind         string            `json:"kind"`
	Evidence     string            `json:"evidence,omitempty"`
	ObservedAt   time.Time         `json:"observed_at"`
}

// RecordObservation appends one observation. Source/trust/confidence must be
// provided by the caller — the store does not synthesize them.
func (s *Store) RecordObservation(o Observation) error {
	coll, err := s.app.Dao().FindCollectionByNameOrId("runtime_observations")
	if err != nil {
		return fmt.Errorf("RecordObservation: collection not found: %w", err)
	}
	if o.ObservedAt.IsZero() {
		o.ObservedAt = time.Now().UTC()
	}
	if o.Confidence < 0 {
		o.Confidence = 0
	}
	if o.Confidence > 1 {
		o.Confidence = 1
	}
	rec := models.NewRecord(coll)
	rec.Set("deployment", o.DeploymentID)
	rec.Set("source", string(o.Source))
	rec.Set("trust", string(o.Trust))
	rec.Set("confidence", o.Confidence)
	rec.Set("kind", o.Kind)
	rec.Set("evidence", o.Evidence)
	// PocketBase 0.22 uses DefaultDateLayout = "2006-01-02 15:04:05.000Z"
	// for date fields. Any other format (RFC3339, RFC3339Nano, time.Time
	// value) silently zero-times on read. Use the exact layout.
	rec.Set("observed_at", pbDate(o.ObservedAt))
	return s.app.Dao().SaveRecord(rec)
}

// ListObservations returns observations for a deployment, newest first.
func (s *Store) ListObservations(deploymentID string, limit int) ([]*Observation, error) {
	if limit <= 0 {
		limit = 100
	}
	recs, err := s.app.Dao().FindRecordsByFilter("runtime_observations",
		"deployment = {:d}", "-observed_at", limit, 0,
		map[string]interface{}{"d": deploymentID})
	if err := emptyOnNoRows(err); err != nil {
		return nil, err
	}
	out := make([]*Observation, 0, len(recs))
	for _, r := range recs {
		out = append(out, &Observation{
			ID:           r.Id,
			DeploymentID: r.GetString("deployment"),
			Source:       ObservationSource(r.GetString("source")),
			Trust:        ObservationTrust(r.GetString("trust")),
			Confidence:   r.GetFloat("confidence"),
			Kind:         r.GetString("kind"),
			Evidence:     r.GetString("evidence"),
			// observed_at is set explicitly during write, but PB v0.22's
			// types.DateTime round-trip is unreliable for user-defined date
			// fields. Fall back to the auto-managed `created` time, which
			// for append-only observations IS the observation time anyway.
			ObservedAt:   r.GetCreated().Time(),
		})
	}
	return out, nil
}
