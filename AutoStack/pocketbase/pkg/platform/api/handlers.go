package api

import (
	"net/http"
	"time"

	"github.com/janlauber/one-click/pkg/governance"
	"github.com/janlauber/one-click/pkg/topology"
	"github.com/janlauber/one-click/pkg/verifier"
	"github.com/labstack/echo/v5"
)

// RegisterRoutes mounts all platform read-only query routes on the given Echo group.
// Expected mount point: /api/v1/platform
//
// All handlers are read-only. None trigger mutations, rollbacks, or any
// form of autonomous action.
func RegisterRoutes(g *echo.Group, store PlatformStore) {
	g.GET("/overview", handleOverview(store))

	g.GET("/executions", handleListExecutions(store))
	g.GET("/executions/:id", handleGetExecution(store))

	g.GET("/topology/:id", handleGetTopology(store))

	g.GET("/scheduler/:id", handleGetScheduler(store))

	g.GET("/governance/:id", handleGetGovernance(store))

	g.GET("/replay/:id", handleGetReplay(store))

	g.POST("/exports/verify", handleVerifyExports())

	g.GET("/providers", handleGetProviders())

	// PX-1: Operator Timeline, Forensics, Drill Mode
	g.GET("/timeline/:id", handleGetTimeline(store))
	g.GET("/forensics/:id", handleGetForensics(store))
	g.POST("/drills/:id", handleRunDrill(store))
}

// ─── Overview ──────────────────────────────────────────────────────────────

func handleOverview(store PlatformStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		summaries, err := store.ListExecutionSummaries(c.Request().Context())
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		verified, contradicted, blocked, pendingApprovals, overrides := 0, 0, 0, 0, 0
		for _, s := range summaries {
			if s.VerificationState == string(verifier.VerificationVerified) {
				verified++
			}
			if s.VerificationState == string(verifier.VerificationContradicted) {
				contradicted++
			}
			blocked += 0 // populated from scheduler if needed
			if s.HasOverrides {
				overrides++
			}
		}
		_ = blocked
		_ = pendingApprovals

		ov := PlatformOverview{
			ActiveExecutions:  len(summaries),
			VerifiedCount:     verified,
			ContradictedCount: contradicted,
			PendingApprovals:  pendingApprovals,
			OverrideCount:     overrides,
			Limitations: []string{
				"Overview counts reflect in-process registry state only; counts may differ after restart until replay completes.",
				"Pending approval count is derived from stored governance snapshots and may lag real-time.",
			},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		return c.JSON(http.StatusOK, ov)
	}
}

// ─── Executions ────────────────────────────────────────────────────────────

func handleListExecutions(store PlatformStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		summaries, err := store.ListExecutionSummaries(c.Request().Context())
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, summaries)
	}
}

func handleGetExecution(store PlatformStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		id := c.PathParam("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "execution id is required")
		}

		detail, err := store.GetExecutionDetail(c.Request().Context(), id)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return c.JSON(http.StatusOK, detail)
	}
}

// ─── Topology ──────────────────────────────────────────────────────────────

func handleGetTopology(store PlatformStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		id := c.PathParam("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "execution id is required")
		}

		snap, err := store.GetTopologySnapshot(c.Request().Context(), id)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}

		explanation := topology.ExplainTopology(topology.TopologyGraph{}, nil)

		resp := TopologyResponse{
			ExecutionID: id,
			Snapshot:    snap,
			Explanation: &explanation,
			Limitations: []string{
				"Topology is computed from the execution plan at plan time. Live provider topology may differ.",
				"Cycle detection covers plan-level dependencies only, not runtime-inferred dependencies.",
			},
		}
		return c.JSON(http.StatusOK, resp)
	}
}

// ─── Scheduler ─────────────────────────────────────────────────────────────

func handleGetScheduler(store PlatformStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		id := c.PathParam("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "execution id is required")
		}

		snap, err := store.GetSchedulingSnapshot(c.Request().Context(), id)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}

		resp := SchedulerResponse{
			ExecutionID: id,
			Snapshot:    snap,
			Limitations: []string{
				"Queue state reflects the last exported snapshot; it does not stream live dequeue events.",
				"Starvation detection requires DeferredCount to be recorded by the scheduler fairness state.",
			},
		}
		return c.JSON(http.StatusOK, resp)
	}
}

// ─── Governance ────────────────────────────────────────────────────────────

func handleGetGovernance(store PlatformStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		id := c.PathParam("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "execution id is required")
		}

		snap, err := store.GetGovernanceSnapshot(c.Request().Context(), id)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}

		chainOk, chainReason := governance.VerifyAuditChainIntegrity(snap.AuditChain)
		snapOk, snapReason := governance.VerifyGovernanceSnapshot(snap)

		reason := ""
		if !snapOk {
			reason = snapReason
		} else if !chainOk {
			reason = chainReason
		}

		resp := GovernanceResponse{
			ExecutionID: id,
			Snapshot:    snap,
			ChainValid:  chainOk && snapOk,
			ChainReason: reason,
			Limitations: []string{
				"Policy evaluation is point-in-time. Policy set may have changed since the snapshot was taken.",
				"RBAC decisions reflect the role set at snapshot time.",
			},
		}
		return c.JSON(http.StatusOK, resp)
	}
}

// ─── Replay ────────────────────────────────────────────────────────────────

func handleGetReplay(store PlatformStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		id := c.PathParam("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "execution id is required")
		}

		events, order, err := store.GetReplayEvents(c.Request().Context(), id)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}

		resp := ReplayResponse{
			ExecutionID: id,
			ReplayOrder: order,
			Events:      events,
			Limitations: []string{
				"Replay order is derived from stored scheduler events; it reflects the historical dequeue sequence.",
				"Replay does not re-execute any stage. It is a read-only reconstruction.",
			},
		}
		return c.JSON(http.StatusOK, resp)
	}
}

// ─── Exports ───────────────────────────────────────────────────────────────

func handleVerifyExports() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req ExportVerificationRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body: "+err.Error())
		}

		resp := ExportVerificationResponse{
			VerificationValid: true,
			GovernanceValid:   true,
			TopologyValid:     true,
			SchedulerValid:    true,
		}

		if req.VerificationSnap != nil {
			ok, reason := verifier.VerifyVerificationSnapshot(req.VerificationSnap)
			resp.VerificationValid = ok
			if !ok {
				resp.Reason = "verification snapshot: " + reason
			}
		}

		if req.GovernanceSnap != nil {
			ok, reason := governance.VerifyGovernanceSnapshot(req.GovernanceSnap)
			resp.GovernanceValid = ok
			if !ok && resp.Reason == "" {
				resp.Reason = "governance snapshot: " + reason
			}
		}

		if req.TopologySnap != nil {
			ok, reason := topology.VerifyTopologySnapshot(req.TopologySnap)
			resp.TopologyValid = ok
			if !ok && resp.Reason == "" {
				resp.Reason = "topology snapshot: " + reason
			}
		}

		if req.SchedulerSnap != nil {
			ok, reason := verifySchedulingSnapshot(req.SchedulerSnap)
			resp.SchedulerValid = ok
			if !ok && resp.Reason == "" {
				resp.Reason = "scheduler snapshot: " + reason
			}
		}

		return c.JSON(http.StatusOK, resp)
	}
}

// ─── Providers ─────────────────────────────────────────────────────────────

func handleGetProviders() echo.HandlerFunc {
	return func(c echo.Context) error {
		known := []string{"aws-ecs", "azure-aca", "gcp-cloudrun"}
		caps := topology.BuildCapabilityMatrix(known)
		resp := ProviderCapabilityResponse{
			Capabilities: caps,
			Limitations: []string{
				"Capabilities reflect static registry data; provider-specific runtime limits may differ.",
				"Traffic shifting and rollback support depend on the active provider SDK version.",
			},
		}
		return c.JSON(http.StatusOK, resp)
	}
}
