package runtime_deploy

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
)

// pbDateLayout matches pocketbase's types.DefaultDateLayout. Any other
// format will silently store as zero on round-trip through GetDateTime.
const pbDateLayout = "2006-01-02 15:04:05.000Z"

// pbDate formats a time.Time in the layout PocketBase expects for date fields.
func pbDate(t time.Time) string {
	return t.UTC().Format(pbDateLayout)
}

// parsePBDate parses a PocketBase-formatted date string. Returns zero time on
// any error (including empty string). PB stores dates in DefaultDateLayout but
// the type wrapping (types.DateTime in m.data) makes GetTime / GetDateTime
// unreliable for user-defined date fields in v0.22 — reading the raw string
// and parsing it ourselves sidesteps the issue.
func parsePBDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{pbDateLayout, time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// emptyOnNoRows treats sql.ErrNoRows as an empty result rather than an error.
// PocketBase's FindRecordsByFilter sometimes wraps that error instead of
// returning a zero-length slice, which spams logs on every engine tick.
func emptyOnNoRows(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

// Store wraps PocketBase access for runtime_deployments and
// runtime_deployment_events collections. All deployment state mutations
// go through this layer so the audit/event trail and the deployment
// record stay consistent.
type Store struct {
	app *pocketbase.PocketBase
}

func NewStore(app *pocketbase.PocketBase) *Store {
	return &Store{app: app}
}

// Create persists a new deployment record in state=planned and emits the
// initial event. Atomicity is best-effort: PocketBase doesn't expose
// multi-collection transactions; the event write is non-blocking but is
// retried by the engine on failure (deployment is the source of truth).
func (s *Store) Create(userID string, req CreateRequest) (*Deployment, error) {
	coll, err := s.app.Dao().FindCollectionByNameOrId("runtime_deployments")
	if err != nil {
		return nil, fmt.Errorf("collection runtime_deployments not found: %w", err)
	}
	rec := models.NewRecord(coll)
	now := time.Now().UTC()
	rec.Set("name", req.Name)
	rec.Set("user", userID)
	rec.Set("image", req.Image)
	rec.Set("host_port", req.HostPort)
	rec.Set("container_port", req.ContainerPort)
	rec.Set("state", string(StatePlanned))
	rec.Set("container_name", deriveContainerName(req.Name))
	if err := s.app.Dao().SaveRecord(rec); err != nil {
		return nil, fmt.Errorf("save deployment: %w", err)
	}
	d := s.recordToDeployment(rec)
	_ = s.appendEvent(d.ID, 1, "", StatePlanned, "created", "deployment created", "", now)
	return d, nil
}

// Get returns a deployment by ID.
func (s *Store) Get(id string) (*Deployment, error) {
	rec, err := s.app.Dao().FindRecordById("runtime_deployments", id)
	if err != nil {
		return nil, err
	}
	return s.recordToDeployment(rec), nil
}

// ListForUser returns deployments owned by a user, newest first.
func (s *Store) ListForUser(userID string) ([]*Deployment, error) {
	recs, err := s.app.Dao().FindRecordsByFilter("runtime_deployments",
		"user = {:u}", "-created", 100, 0,
		map[string]interface{}{"u": userID})
	if err := emptyOnNoRows(err); err != nil {
		return nil, err
	}
	out := make([]*Deployment, 0, len(recs))
	for _, r := range recs {
		out = append(out, s.recordToDeployment(r))
	}
	return out, nil
}

// ListByState returns deployments in any of the given states. Used by the
// engine worker to find deployments needing further processing.
func (s *Store) ListByState(states ...DeploymentState) ([]*Deployment, error) {
	if len(states) == 0 {
		return nil, nil
	}
	filter := ""
	params := map[string]interface{}{}
	for i, st := range states {
		key := fmt.Sprintf("s%d", i)
		if filter != "" {
			filter += " || "
		}
		filter += "state = {:" + key + "}"
		params[key] = string(st)
	}
	recs, err := s.app.Dao().FindRecordsByFilter("runtime_deployments",
		filter, "-created", 100, 0, params)
	if err := emptyOnNoRows(err); err != nil {
		return nil, err
	}
	out := make([]*Deployment, 0, len(recs))
	for _, r := range recs {
		out = append(out, s.recordToDeployment(r))
	}
	return out, nil
}

// Transition atomically updates the deployment's state, mutates any fields
// the caller wants to set, and appends the matching audit/replay event.
// `mutate` may be nil if only the state is changing.
func (s *Store) Transition(id string, from, to DeploymentState, eventType, detail, errMsg string, mutate func(rec *models.Record)) error {
	rec, err := s.app.Dao().FindRecordById("runtime_deployments", id)
	if err != nil {
		return fmt.Errorf("transition: load deployment: %w", err)
	}
	current := DeploymentState(rec.GetString("state"))
	if from != "" && current != from {
		return fmt.Errorf("transition refused: deployment %s is in state %q, expected %q", id, current, from)
	}
	rec.Set("state", string(to))
	if errMsg != "" {
		rec.Set("last_error", errMsg)
	} else {
		// Clear stale error on successful forward transitions.
		rec.Set("last_error", "")
	}
	if mutate != nil {
		mutate(rec)
	}
	if err := s.app.Dao().SaveRecord(rec); err != nil {
		return fmt.Errorf("transition save: %w", err)
	}
	seq := s.nextSequence(id)
	now := time.Now().UTC()
	return s.appendEvent(id, seq, current, to, eventType, detail, errMsg, now)
}

// AppendEvent writes a standalone event (not coupled to a state change).
// Used for log snapshots, drift observations, etc.
func (s *Store) AppendEvent(deploymentID, eventType, detail string) error {
	seq := s.nextSequence(deploymentID)
	curr, _ := s.Get(deploymentID)
	st := DeploymentState("")
	if curr != nil {
		st = curr.State
	}
	return s.appendEvent(deploymentID, seq, st, st, eventType, detail, "", time.Now().UTC())
}

// Events returns the full event log for a deployment, ordered by sequence.
func (s *Store) Events(deploymentID string) ([]*DeploymentEvent, error) {
	recs, err := s.app.Dao().FindRecordsByFilter("runtime_deployment_events",
		"deployment = {:d}", "sequence", 1000, 0,
		map[string]interface{}{"d": deploymentID})
	if err := emptyOnNoRows(err); err != nil {
		return nil, err
	}
	out := make([]*DeploymentEvent, 0, len(recs))
	for _, r := range recs {
		out = append(out, &DeploymentEvent{
			ID:           r.Id,
			DeploymentID: r.GetString("deployment"),
			Sequence:     r.GetInt("sequence"),
			FromState:    DeploymentState(r.GetString("from_state")),
			ToState:      DeploymentState(r.GetString("to_state")),
			EventType:    r.GetString("event_type"),
			Detail:       r.GetString("detail"),
			Error:        r.GetString("error"),
			// Same PB v0.22 date round-trip workaround as observations.
			OccurredAt:   r.GetCreated().Time(),
		})
	}
	return out, nil
}

// nextSequence returns the next event sequence number for a deployment.
// Worst case is a race that yields a duplicate sequence; the events table
// does NOT enforce uniqueness on (deployment, sequence) so a duplicate is
// observable but non-fatal. The engine is single-worker so races are rare.
func (s *Store) nextSequence(deploymentID string) int {
	recs, err := s.app.Dao().FindRecordsByFilter("runtime_deployment_events",
		"deployment = {:d}", "-sequence", 1, 0,
		map[string]interface{}{"d": deploymentID})
	if err != nil || len(recs) == 0 {
		return 1
	}
	return recs[0].GetInt("sequence") + 1
}

func (s *Store) appendEvent(deploymentID string, seq int, from, to DeploymentState, eventType, detail, errMsg string, when time.Time) error {
	coll, err := s.app.Dao().FindCollectionByNameOrId("runtime_deployment_events")
	if err != nil {
		return fmt.Errorf("event collection not found: %w", err)
	}
	rec := models.NewRecord(coll)
	rec.Set("deployment", deploymentID)
	rec.Set("sequence", seq)
	rec.Set("from_state", string(from))
	rec.Set("to_state", string(to))
	rec.Set("event_type", eventType)
	rec.Set("detail", detail)
	rec.Set("error", errMsg)
	rec.Set("occurred_at", pbDate(when))
	return s.app.Dao().SaveRecord(rec)
}

func (s *Store) recordToDeployment(rec *models.Record) *Deployment {
	startedAt := parsePBDate(rec.GetString("started_at"))
	d := &Deployment{
		ID:                rec.Id,
		Name:              rec.GetString("name"),
		UserID:            rec.GetString("user"),
		Image:             rec.GetString("image"),
		HostPort:          rec.GetInt("host_port"),
		ContainerPort:     rec.GetInt("container_port"),
		State:             DeploymentState(rec.GetString("state")),
		PreviousImage:     rec.GetString("previous_image"),
		ContainerID:       rec.GetString("container_id"),
		ContainerName:     rec.GetString("container_name"),
		HealthURL:         rec.GetString("health_url"),
		LastError:         rec.GetString("last_error"),
		StartedAt:         startedAt,
		LastHealthCheckAt: parsePBDate(rec.GetString("last_health_check_at")),
		RestartCount:      rec.GetInt("restart_count"),
		CreatedAt:         rec.GetCreated().Time(),
		UpdatedAt:         rec.GetUpdated().Time(),
	}
	// uptime_seconds is computed at read time: only meaningful when the
	// container is running (certified) AND we have a started_at.
	if !startedAt.IsZero() && d.State == StateCertified {
		d.UptimeSeconds = int64(time.Since(startedAt).Seconds())
	}
	return d
}

// deriveContainerName produces a Docker-safe container name from the
// user-supplied deployment name. Adds an autostack- prefix and replaces
// any non [a-zA-Z0-9_.-] characters with '-'.
func deriveContainerName(name string) string {
	prefix := "autostack-rt-"
	out := make([]byte, 0, len(name)+len(prefix))
	out = append(out, prefix...)
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '.', c == '-':
			out = append(out, c)
		case c == ' ':
			out = append(out, '-')
		}
	}
	return string(out)
}
