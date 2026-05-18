package api

import (
	"context"
	"net/http"

	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/labstack/echo/v5"
)

// StartRequest is the body for POST /execution/start.
type StartRequest struct {
	ExecutionID string                         `json:"execution_id"`
	HolderID    string                         `json:"holder_id"`
	Snapshot    *orchestrator.ExecutionSnapshot `json:"snapshot"`
}

// ApprovalRequest is the body for POST /execution/:id/approve and /reject.
type ApprovalRequest struct {
	GateID       string `json:"gate_id"`
	StageID      string `json:"stage_id"`
	OperatorID   string `json:"operator_id"`
	OperatorRole string `json:"operator_role"`
	Reason       string `json:"reason"`
}

// PauseRequest is the body for POST /execution/:id/pause.
type PauseAPIRequest struct {
	OperatorID string `json:"operator_id"`
	Reason     string `json:"reason"`
}

// ResumeRequest is the body for POST /execution/:id/resume.
type ResumeAPIRequest struct {
	OperatorID string `json:"operator_id"`
}

// CancelRequest is the body for POST /execution/:id/cancel.
type CancelRequest struct {
	OperatorID string `json:"operator_id"`
	Reason     string `json:"reason"`
}

// ResolveAmbiguityRequest is the body for POST /execution/:id/resolve-ambiguity.
type ResolveAmbiguityRequest struct {
	AmbiguityID string `json:"ambiguity_id"`
	OperatorID  string `json:"operator_id"`
	Resolution  string `json:"resolution"`
}

// RegisterRoutes mounts execution control-plane routes on the given Echo group.
// All routes require operator action — no automatic progression occurs.
func RegisterRoutes(g *echo.Group, exec *executor.Executor, registry *ExecutionRegistry, journalStore executor.JournalStore, approvalStore executor.ApprovalStore, ambiguityStore executor.AmbiguityStore) {
	g.POST("/start", handleStart(exec, registry))
	g.POST("/:id/advance", handleAdvance(registry))
	g.POST("/:id/approve", handleApprove(exec, registry))
	g.POST("/:id/reject", handleReject(exec, registry))
	g.POST("/:id/pause", handlePause(registry))
	g.POST("/:id/resume", handleResume(registry))
	g.POST("/:id/cancel", handleCancel(registry))
	g.POST("/:id/resolve-ambiguity", handleResolveAmbiguity(exec, registry))
	g.GET("/:id", handleGetState(journalStore, approvalStore, ambiguityStore))
	g.GET("/:id/snapshot", handleGetSnapshot(registry))
}

// handleStart creates and starts a new execution from an operator-supplied snapshot.
func handleStart(exec *executor.Executor, registry *ExecutionRegistry) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req StartRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		}
		if req.ExecutionID == "" || req.Snapshot == nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "execution_id and snapshot are required"})
		}

		rt, err := exec.Prepare(req.ExecutionID, req.HolderID, req.Snapshot)
		if err != nil {
			return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		}
		if err := rt.Start(); err != nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		}
		registry.Register(rt)

		return c.JSON(http.StatusCreated, map[string]interface{}{
			"execution_id": req.ExecutionID,
			"state":        rt.State(),
		})
	}
}

// handleAdvance requests the next stage be executed. Operator-initiated only.
func handleAdvance(registry *ExecutionRegistry) echo.HandlerFunc {
	return func(c echo.Context) error {
		rt, err := registry.MustGet(c.PathParam("id"))
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		if err := rt.Advance(context.Background()); err != nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"state": string(rt.State())})
	}
}

// handleApprove records an operator approval for a gate.
func handleApprove(exec *executor.Executor, registry *ExecutionRegistry) echo.HandlerFunc {
	return func(c echo.Context) error {
		executionID := c.PathParam("id")
		var req ApprovalRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		d := executor.ApprovalDecision{
			ExecutionID:  executionID,
			GateID:       req.GateID,
			StageID:      req.StageID,
			OperatorID:   req.OperatorID,
			OperatorRole: req.OperatorRole,
			Reason:       req.Reason,
			Approved:     true,
		}
		if err := exec.RecordApproval(d); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"gate_id": req.GateID, "approved": "true"})
	}
}

// handleReject records an operator rejection for a gate.
func handleReject(exec *executor.Executor, registry *ExecutionRegistry) echo.HandlerFunc {
	return func(c echo.Context) error {
		executionID := c.PathParam("id")
		var req ApprovalRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		d := executor.ApprovalDecision{
			ExecutionID:  executionID,
			GateID:       req.GateID,
			StageID:      req.StageID,
			OperatorID:   req.OperatorID,
			OperatorRole: req.OperatorRole,
			Reason:       req.Reason,
			Approved:     false,
		}
		if err := exec.RecordRejection(d); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"gate_id": req.GateID, "approved": "false"})
	}
}

// handlePause requests an operator-initiated pause.
func handlePause(registry *ExecutionRegistry) echo.HandlerFunc {
	return func(c echo.Context) error {
		rt, err := registry.MustGet(c.PathParam("id"))
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		var req PauseAPIRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		if err := rt.Pause(req.OperatorID, req.Reason); err != nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"state": string(rt.State())})
	}
}

// handleResume requests an operator-initiated resume.
func handleResume(registry *ExecutionRegistry) echo.HandlerFunc {
	return func(c echo.Context) error {
		rt, err := registry.MustGet(c.PathParam("id"))
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		var req ResumeAPIRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		if err := rt.Resume(req.OperatorID); err != nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"state": string(rt.State())})
	}
}

// handleCancel requests an operator-initiated cancellation.
func handleCancel(registry *ExecutionRegistry) echo.HandlerFunc {
	return func(c echo.Context) error {
		rt, err := registry.MustGet(c.PathParam("id"))
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		var req CancelRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		if err := rt.Cancel(req.OperatorID, req.Reason); err != nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"state": string(rt.State())})
	}
}

// handleResolveAmbiguity records an operator resolution for a named ambiguity.
func handleResolveAmbiguity(exec *executor.Executor, registry *ExecutionRegistry) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req ResolveAmbiguityRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		if err := exec.ResolveAmbiguity(req.AmbiguityID, req.OperatorID, req.Resolution); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		// After resolving the ambiguity record, transition the runtime FSM.
		rt, err := registry.MustGet(c.PathParam("id"))
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		if err := rt.ResolveAmbiguity(req.OperatorID, req.Resolution); err != nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"state": string(rt.State())})
	}
}

// handleGetState returns a replay-derived view of the execution state.
// This can be called even if the runtime is not in the registry (crash recovery path).
func handleGetState(journalStore executor.JournalStore, approvalStore executor.ApprovalStore, ambiguityStore executor.AmbiguityStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		executionID := c.PathParam("id")
		snapshot := c.QueryParam("snapshot_json") // future: load from store by ID
		_ = snapshot
		// For now return 400 if snapshot is not provided — full replay requires a snapshot.
		// In production, snapshots would be retrieved from PocketBase by executionID.
		return c.JSON(http.StatusOK, map[string]string{
			"execution_id": executionID,
			"note":         "GET /execution/:id replay requires snapshot retrieval from PocketBase — not yet wired in this phase",
		})
	}
}

// handleGetSnapshot returns the tamper-evident runtime snapshot.
func handleGetSnapshot(registry *ExecutionRegistry) echo.HandlerFunc {
	return func(c echo.Context) error {
		rt, err := registry.MustGet(c.PathParam("id"))
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		snap := executor.ExportRuntimeSnapshot(
			rt.ExecutionID(),
			rt.Snapshot(),
			rt.State(),
			rt.CurrentStageIndex(),
			rt.Transitions(),
			rt.StageRecords(),
			nil, // ambiguityIDs — requires ambiguity store lookup
			nil, // rollback recommendations
			rt.RollbackRecommendations(),
			rt.CompensationRecommendations(),
		)
		return c.JSON(http.StatusOK, snap)
	}
}
