package controller

import (
	"github.com/janlauber/one-click/pkg/runtime_deploy"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
)

// activeRuntimeEngine is set by main.go after the engine is constructed and
// started. Singleton-by-design; one engine per process.
var activeRuntimeEngine *runtime_deploy.Engine
var activeRuntimeStore *runtime_deploy.Store
var activeRuntimeDriver *runtime_deploy.DockerDriver

// SetActiveRuntime publishes the engine/store/driver triple to the package.
// Called once at boot.
func SetActiveRuntime(engine *runtime_deploy.Engine, store *runtime_deploy.Store, driver *runtime_deploy.DockerDriver) {
	activeRuntimeEngine = engine
	activeRuntimeStore = store
	activeRuntimeDriver = driver
}

// HandleRuntimeHealth reports whether the engine can drive deployments.
// GET /api/v1/runtime/health
func HandleRuntimeHealth(c echo.Context, _ *pocketbase.PocketBase) error {
	if activeRuntimeEngine == nil {
		return c.JSON(503, map[string]interface{}{
			"available": false,
			"reason":    "runtime engine not initialized",
		})
	}
	ok, reason := activeRuntimeEngine.Health()
	return c.JSON(200, map[string]interface{}{
		"available": ok,
		"reason":    reason,
	})
}

// HandleRuntimeDeploymentCreate creates a new deployment and enqueues it.
// POST /api/v1/runtime/deployments
func HandleRuntimeDeploymentCreate(c echo.Context, _ *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	if activeRuntimeStore == nil {
		return c.JSON(503, map[string]string{"error": "runtime engine not initialized"})
	}

	var req runtime_deploy.CreateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request"})
	}
	if req.Name == "" || req.Image == "" {
		return c.JSON(400, map[string]string{"error": "name and image are required"})
	}

	d, err := activeRuntimeStore.Create(user.Id, req)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(201, d)
}

// HandleRuntimeDeploymentList lists the operator's deployments.
// GET /api/v1/runtime/deployments
func HandleRuntimeDeploymentList(c echo.Context, _ *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	if activeRuntimeStore == nil {
		return c.JSON(503, map[string]string{"error": "runtime engine not initialized"})
	}
	list, err := activeRuntimeStore.ListForUser(user.Id)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	if list == nil {
		list = []*runtime_deploy.Deployment{}
	}
	return c.JSON(200, list)
}

// HandleRuntimeDeploymentGet returns one deployment with its current state.
// GET /api/v1/runtime/deployments/:id
func HandleRuntimeDeploymentGet(c echo.Context, _ *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	if activeRuntimeStore == nil {
		return c.JSON(503, map[string]string{"error": "runtime engine not initialized"})
	}
	d, err := activeRuntimeStore.Get(c.PathParam("id"))
	if err != nil {
		return c.JSON(404, map[string]string{"error": "deployment not found"})
	}
	if d.UserID != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}
	return c.JSON(200, d)
}

// HandleRuntimeDeploymentEvents returns the full event log for a deployment.
// GET /api/v1/runtime/deployments/:id/events
func HandleRuntimeDeploymentEvents(c echo.Context, _ *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	if activeRuntimeStore == nil {
		return c.JSON(503, map[string]string{"error": "runtime engine not initialized"})
	}
	id := c.PathParam("id")
	d, err := activeRuntimeStore.Get(id)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "deployment not found"})
	}
	if d.UserID != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}
	events, err := activeRuntimeStore.Events(id)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	if events == nil {
		events = []*runtime_deploy.DeploymentEvent{}
	}
	return c.JSON(200, events)
}

// HandleRuntimeDeploymentLogs streams (well, snapshots) container logs.
// GET /api/v1/runtime/deployments/:id/logs?tail=N
func HandleRuntimeDeploymentLogs(c echo.Context, _ *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	if activeRuntimeStore == nil || activeRuntimeDriver == nil {
		return c.JSON(503, map[string]string{"error": "runtime engine not initialized"})
	}
	id := c.PathParam("id")
	d, err := activeRuntimeStore.Get(id)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "deployment not found"})
	}
	if d.UserID != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}
	if d.ContainerID == "" {
		return c.JSON(200, map[string]string{"logs": "(no container yet)"})
	}
	tail := 200
	logs, err := activeRuntimeDriver.Logs(c.Request().Context(), d.ContainerID, tail)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, map[string]string{"logs": logs})
}

// HandleRuntimeDeploymentRollback transitions the deployment into rollback.
// POST /api/v1/runtime/deployments/:id/rollback
func HandleRuntimeDeploymentRollback(c echo.Context, _ *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	if activeRuntimeStore == nil || activeRuntimeEngine == nil {
		return c.JSON(503, map[string]string{"error": "runtime engine not initialized"})
	}
	id := c.PathParam("id")
	d, err := activeRuntimeStore.Get(id)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "deployment not found"})
	}
	if d.UserID != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}
	var req runtime_deploy.RollbackRequest
	_ = c.Bind(&req)
	if err := activeRuntimeEngine.RequestRollback(d, req); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	return c.JSON(202, map[string]string{"status": "rollback queued"})
}

// HandleRuntimeDeploymentObservations returns the append-only observation log.
// GET /api/v1/runtime/deployments/:id/observations
func HandleRuntimeDeploymentObservations(c echo.Context, _ *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	if activeRuntimeStore == nil {
		return c.JSON(503, map[string]string{"error": "runtime engine not initialized"})
	}
	id := c.PathParam("id")
	d, err := activeRuntimeStore.Get(id)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "deployment not found"})
	}
	if d.UserID != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}
	obs, err := activeRuntimeStore.ListObservations(id, 200)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	if obs == nil {
		obs = []*runtime_deploy.Observation{}
	}
	// Include the current lease holder for operator visibility.
	holder, live, _ := activeRuntimeStore.LeaseHolder(id)
	return c.JSON(200, map[string]interface{}{
		"observations":  obs,
		"lease_holder":  holder,
		"lease_live":    live,
		"this_worker":   runtime_deploy.WorkerID(),
	})
}

// HandleRuntimeDeploymentReplay reconstructs the lifecycle from the event chain
// and returns the deterministic replay result (final state + chain hash).
// GET /api/v1/runtime/deployments/:id/replay
func HandleRuntimeDeploymentReplay(c echo.Context, _ *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	if activeRuntimeStore == nil {
		return c.JSON(503, map[string]string{"error": "runtime engine not initialized"})
	}
	id := c.PathParam("id")
	d, err := activeRuntimeStore.Get(id)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "deployment not found"})
	}
	if d.UserID != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}
	events, err := activeRuntimeStore.Events(id)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	result, replayErr := runtime_deploy.Replay(events)
	resp := map[string]interface{}{
		"deployment_id": id,
		"event_count":   len(events),
		"replay":        result,
	}
	if replayErr != nil {
		resp["replay_error"] = replayErr.Error()
		return c.JSON(422, resp)
	}
	return c.JSON(200, resp)
}

// HandleRuntimeDeploymentDestroy removes the container and marks destroyed.
// DELETE /api/v1/runtime/deployments/:id
func HandleRuntimeDeploymentDestroy(c echo.Context, _ *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	if activeRuntimeStore == nil || activeRuntimeEngine == nil {
		return c.JSON(503, map[string]string{"error": "runtime engine not initialized"})
	}
	id := c.PathParam("id")
	d, err := activeRuntimeStore.Get(id)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "deployment not found"})
	}
	if d.UserID != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}
	reason := c.QueryParam("reason")
	if reason == "" {
		reason = "operator-initiated destroy"
	}
	if err := activeRuntimeEngine.Destroy(c.Request().Context(), d, reason); err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, map[string]string{"status": "destroyed"})
}
