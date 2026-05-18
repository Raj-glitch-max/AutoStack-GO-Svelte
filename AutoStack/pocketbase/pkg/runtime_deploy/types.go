// Package runtime_deploy implements a REAL local-Docker deployment lifecycle.
//
// It is intentionally NOT abstract. There is no provider plugin layer here.
// The single execution target is local Docker (via the `docker` CLI). That
// constraint is what makes this package honest: every state transition is a
// real subprocess that either succeeds or fails, and the resulting runtime
// truth is persisted before the next transition runs.
//
// Cloud provider integration lives in pkg/providers and is intentionally
// separate — see docs/runtime-architecture.md.
//
// State machine (forward-only except for rollback paths):
//
//   planned → validating → provisioning → deploying → verifying →
//     stabilized → certified
//                         │
//                         └─ contradicted (drift detected mid-flight)
//                         └─ failed (terminal)
//                         └─ rollback_pending → rolling_back → rolled_back
//                         └─ destroyed (operator explicit removal)
//
// Every transition writes one append-only event to RuntimeDeploymentEvents.
package runtime_deploy

import "time"

// DeploymentState is the lifecycle state of a runtime deployment.
type DeploymentState string

const (
	StatePlanned         DeploymentState = "planned"
	StateValidating      DeploymentState = "validating"
	StateProvisioning    DeploymentState = "provisioning"
	StateDeploying       DeploymentState = "deploying"
	StateVerifying       DeploymentState = "verifying"
	StateStabilized      DeploymentState = "stabilized"
	StateCertified       DeploymentState = "certified"
	StateContradicted    DeploymentState = "contradicted"
	StateRollbackPending DeploymentState = "rollback_pending"
	StateRollingBack     DeploymentState = "rolling_back"
	StateRolledBack      DeploymentState = "rolled_back"
	StateFailed          DeploymentState = "failed"
	StateDestroyed       DeploymentState = "destroyed"
)

// TerminalStates are end-of-life states the engine will not transition out of
// without explicit operator action.
var TerminalStates = map[DeploymentState]bool{
	StateCertified:    true,
	StateFailed:       true,
	StateRolledBack:   true,
	StateDestroyed:    true,
	StateContradicted: false, // remains observable but engine stops auto-driving
}

// Deployment is the current state of a runtime deployment. This is the
// "desired + observed snapshot" record persisted in PocketBase.
type Deployment struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	UserID        string          `json:"user_id"`
	Image         string          `json:"image"`
	HostPort      int             `json:"host_port"`
	ContainerPort int             `json:"container_port"`
	State         DeploymentState `json:"state"`
	PreviousImage string          `json:"previous_image,omitempty"` // for rollback
	ContainerID   string          `json:"container_id,omitempty"`
	ContainerName string          `json:"container_name,omitempty"`
	HealthURL     string          `json:"health_url,omitempty"`
	LastError     string          `json:"last_error,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// DeploymentEvent is one append-only event in the deployment's lifecycle log.
// Events are never updated or deleted. They serve as the audit + replay trail.
type DeploymentEvent struct {
	ID           string          `json:"id"`
	DeploymentID string          `json:"deployment_id"`
	Sequence     int             `json:"sequence"`
	FromState    DeploymentState `json:"from_state"`
	ToState      DeploymentState `json:"to_state"`
	EventType    string          `json:"event_type"`
	Detail       string          `json:"detail"`
	Error        string          `json:"error,omitempty"`
	OccurredAt   time.Time       `json:"occurred_at"`
}

// CreateRequest is the input for a new deployment.
type CreateRequest struct {
	Name          string `json:"name"`
	Image         string `json:"image"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
}

// RollbackRequest carries the image to roll back to (defaults to PreviousImage).
type RollbackRequest struct {
	TargetImage string `json:"target_image,omitempty"`
	Reason      string `json:"reason"`
}
