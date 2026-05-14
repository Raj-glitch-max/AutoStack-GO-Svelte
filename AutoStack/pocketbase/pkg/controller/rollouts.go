package controller

import (
	"log"
	"strings"
	"time"

	"github.com/janlauber/one-click/pkg/k8s"
	"github.com/janlauber/one-click/pkg/util"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	pbmodels "github.com/pocketbase/pocketbase/models"
)

// isCloudTargetType reports whether a rollout's target_type indicates a
// cloud provider (vs. Kubernetes). Empty string is treated as "kubernetes"
// for backwards-compat with rollouts created before the field existed.
func isCloudTargetType(t string) bool {
	switch t {
	case "cloudrun", "ecs", "aca":
		return true
	default:
		return false
	}
}

// targetTypeToTargetProvider maps a rollout's target_type to the value
// stored in deployment_targets.provider. The two enums differ by design:
// the rollout describes the *workload class* (cloudrun); the target
// describes the *concrete provider* (gcp-cloudrun).
func targetTypeToTargetProvider(t string) string {
	switch t {
	case "cloudrun":
		return "gcp-cloudrun"
	case "ecs":
		return "aws-ecs"
	case "aca":
		return "azure-aca"
	default:
		return ""
	}
}

// flipCloudTargetsToPendingOnRespec flips every deployment_targets row
// for a cloud rollout from `running`/`updating`/`error` back to `pending`
// so the reconciler dispatcher re-deploys with the new manifest.
//
// Targets with an in-flight operation are left alone — the dispatcher's
// stale-spec check (rolloutMovedSince) will detect the manifest change
// when it returns and re-dispatch on the next cycle without our help.
//
// Targets in terminal `deleted` or `deleting` state are also left alone:
// a respec on an ending rollout is operator-confusion territory that we
// don't try to resolve automatically.
func flipCloudTargetsToPendingOnRespec(app *pocketbase.PocketBase, rolloutID string) error {
	targets, err := app.Dao().FindRecordsByFilter(
		"deployment_targets",
		"rollout = {:rollout}",
		"", 50, 0,
		map[string]interface{}{"rollout": rolloutID},
	)
	if err != nil {
		return err
	}
	for _, t := range targets {
		if t.GetString("current_operation") != "" {
			log.Printf("[CLOUD_RESPEC_DEFER] target=%s reason=in_flight_operation", t.Id)
			// The dispatcher's stale-spec check (rolloutMovedSince) will
			// detect the change when it returns and re-dispatch from
			// pending. No additional flag needed here.
			continue
		}
		switch t.GetString("status") {
		case "deleted", "deleting", "pending":
			continue
		}
		t.Set("status", "pending")
		t.Set("last_state_change_at", time.Now().UTC().Format(time.RFC3339))
		if err := app.Dao().SaveRecord(t); err != nil {
			return err
		}
		// Phase 2.3 intent-boundary lineage: the operator's respec is what
		// caused the redispatch. The dispatcher's later in_progress row will
		// say "updated" without context; this row says why.
		writeIntentHistory(app, rolloutID, t.Id, "updated",
			"manifest changed; redispatch queued", t.GetString("provider"))
	}
	return nil
}

// markCloudTargetForDestroy flips the deployment_targets row for a
// cloud rollout into status=deleting, signaling the reconciler to
// dispatch Provider.Destroy on the next tick. If no target row exists
// (the row was never created — e.g., a rollout that never reached the
// dispatcher) this is a no-op success.
//
// Phase 2.3 re-arm: when a target has an in-flight current_operation,
// we cannot flip status (the dispatcher's release path would race the
// write). Instead we set `pending_destroy = true`. The dispatcher's
// post-Deploy success path checks this flag and routes the target to
// `deleting` on release, so the destroy intent is not silently lost.
//
// Phase 2.3 intent-boundary lineage: we write a deployment_history row
// recording the operator's destroy intent (action=deleted,
// status=in_progress, message=intent) BEFORE the reconciler dispatches.
// This way the timeline starts at "operator asked to delete" rather than
// at "dispatcher claimed and tried".
func markCloudTargetForDestroy(app *pocketbase.PocketBase, rolloutID string) error {
	targets, err := app.Dao().FindRecordsByFilter(
		"deployment_targets",
		"rollout = {:rollout}",
		"", 10, 0,
		map[string]interface{}{"rollout": rolloutID},
	)
	if err != nil {
		return err
	}
	for _, t := range targets {
		// Don't re-mark already-terminal targets.
		switch t.GetString("status") {
		case "deleted", "deleting":
			continue
		}

		if t.GetString("current_operation") != "" {
			// In-flight: set pending_destroy so the dispatcher's release
			// path honors the intent. Do NOT flip status (the release
			// path would race the write).
			t.Set("pending_destroy", true)
			if err := app.Dao().SaveRecord(t); err != nil {
				return err
			}
			log.Printf("[CLOUD_DESTROY_PENDING] target=%s reason=in_flight_operation_will_rearm_on_release", t.Id)
			writeIntentHistory(app, rolloutID, t.Id, "deleted",
				"destroy intent recorded; will dispatch after in-flight operation completes",
				t.GetString("provider"))
			continue
		}

		t.Set("status", "deleting")
		t.Set("last_state_change_at", time.Now().UTC().Format(time.RFC3339))
		if err := app.Dao().SaveRecord(t); err != nil {
			return err
		}
		writeIntentHistory(app, rolloutID, t.Id, "deleted",
			"destroy intent recorded; reconciler will dispatch on next cycle",
			t.GetString("provider"))
	}
	return nil
}

// writeIntentHistory writes a deployment_history row at an operator-facing
// intent boundary (target creation, respec redispatch, destroy mark).
// Phase 2.3: closes the lineage gap where the first history row was the
// dispatcher's in-progress entry, not the operator's intent.
//
// Best-effort: a write failure here logs but does not propagate. Lineage
// is forensic, not load-bearing for correctness.
func writeIntentHistory(app *pocketbase.PocketBase, rolloutID, targetID, action, message, provider string) {
	col, err := app.Dao().FindCollectionByNameOrId("deployment_history")
	if err != nil {
		log.Printf("[INTENT_HISTORY_ERR] collection: %v", err)
		return
	}
	rec := pbmodels.NewRecord(col)
	rec.Set("rollout", rolloutID)
	if targetID != "" {
		rec.Set("target", targetID)
	}
	rec.Set("action", action)
	rec.Set("status", "in_progress")
	rec.Set("message", message)
	if provider != "" {
		rec.Set("provider", provider)
	}
	if err := app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[INTENT_HISTORY_ERR] save: %v", err)
	}
}

// createPendingDeploymentTarget inserts a deployment_targets row that the
// reconciler dispatcher will pick up and execute. Returns an error if the
// rollout is malformed for cloud deployment (missing cloud_account, etc.).
//
// We do NOT call Provider.Deploy here. The dispatcher in pkg/reconciler/
// owns Deploy invocation; doing it inline in the HTTP handler would (a)
// block the request for minutes, (b) lose restart-safety, (c) re-create
// the duplicate-dispatch hazard the operations CAS exists to prevent.
func createPendingDeploymentTarget(app *pocketbase.PocketBase, rolloutID, targetType string, e *core.RecordCreateEvent) error {
	cloudAccountID := e.Record.GetString("cloud_account")
	if cloudAccountID == "" {
		return echo.NewHTTPError(400, "cloud_account is required for cloud rollouts")
	}
	cloudAccount, err := app.Dao().FindRecordById("cloud_accounts", cloudAccountID)
	if err != nil {
		return echo.NewHTTPError(400, "cloud_account not found")
	}
	region := cloudAccount.GetString("region")
	if region == "" {
		return echo.NewHTTPError(400, "cloud_account has no region")
	}
	providerName := targetTypeToTargetProvider(targetType)
	if providerName == "" {
		return echo.NewHTTPError(400, "unrecognized target_type")
	}

	col, err := app.Dao().FindCollectionByNameOrId("deployment_targets")
	if err != nil {
		return err
	}
	rec := pbmodels.NewRecord(col)
	rec.Set("rollout", rolloutID)
	rec.Set("cloud_account", cloudAccountID)
	rec.Set("provider", providerName)
	rec.Set("region", region)
	rec.Set("status", "pending")
	rec.Set("last_state_change_at", time.Now().UTC().Format(time.RFC3339))
	if err := app.Dao().SaveRecord(rec); err != nil {
		return err
	}
	log.Printf("[CLOUD_TARGET_CREATED] rollout=%s target=%s provider=%s region=%s", rolloutID, rec.Id, providerName, region)
	// Phase 2.3 intent-boundary lineage: record the target's creation
	// before the dispatcher's first claim. Operators reconstructing a
	// timeline now see "target provisioned" as the first row.
	writeIntentHistory(app, rolloutID, rec.Id, "created",
		"deployment target created; awaiting reconciler dispatch", providerName)
	return nil
}

func HandleRolloutCreate(e *core.RecordCreateEvent, app *pocketbase.PocketBase) error {

	// Get project
	project, err := app.Dao().FindRecordById("projects", e.Record.GetString("project"))
	if err != nil {
		return err
	}

	// Get deployment
	deployment, err := app.Dao().FindRecordById("deployments", e.Record.GetString("deployment"))
	if err != nil {
		return err
	}

	// Get user
	user, err := app.Dao().FindRecordById("users", project.GetString("user"))
	if err != nil {
		return err
	}

	// Generate a rolloutId (15 characters)
	rolloutId := util.GenerateId(15)

	// Check if endDate is set, if yes, throw error
	if e.Record.GetString("endDate") != "" {
		return echo.NewHTTPError(400, "endDate is not allowed on create")
	}

	// Check if there is another rollout in the same project with no endDate
	running_rollout, err := app.Dao().FindFirstRecordByFilter("rollouts", "endDate = '' && project = {:project} && deployment = {:deployment}",
		dbx.Params{"project": e.Record.GetString("project"), "deployment": e.Record.GetString("deployment")},
	)
	if err != nil {
		if contains := strings.Contains(err.Error(), "no rows"); !contains {
			return err
		}
	}

	// if there is another rollout in the same project with no endDate, set endDate to now on that rollout
	if running_rollout != nil {
		running_rollout.Set("endDate", time.Now().UTC().Format(time.RFC3339))
		err = app.Dao().SaveRecord(running_rollout)
		if err != nil {
			return err
		}
	}

	targetType := e.Record.GetString("target_type")

	// Branch by target_type:
	//   - kubernetes (or unset): legacy path — call the operator-watched CRD
	//     handler unchanged. The Kubernetes path is intentionally untouched.
	//   - cloud target type: create a pending deployment_targets row; the
	//     reconciler dispatcher will pick it up. Do NOT create a Kubernetes
	//     CRD for cloud rollouts — that would attempt to run the same
	//     workload twice and contradict the target_type contract.
	if isCloudTargetType(targetType) {
		if err := createPendingDeploymentTarget(app, rolloutId, targetType, e); err != nil {
			log.Printf("[CLOUD_TARGET_CREATE_FAIL] rollout=%s err=%v", rolloutId, err)
			return err
		}
		// Reuse project/deployment to satisfy unused-import linters in the
		// kubernetes branch above; the cloud branch doesn't need them.
		_ = project
		_ = deployment
		_ = user
	} else {
		// Create rollout in k8s
		err = k8s.CreateOrUpdateRollout(rolloutId, user, project.Id, deployment.Id, e.Record.GetString("manifest"))
		if err != nil {
			log.Println(err)
			return err
		}
	}

	// Update rollout with startDate time.Now().UTC().Format(time.RFC3339)
	e.Record.Set("startDate", time.Now().UTC().Format(time.RFC3339))

	// Update rollout with rolloutId
	e.Record.Set("id", rolloutId)

	return nil

}

func HandleRolloutUpdate(e *core.RecordUpdateEvent, app *pocketbase.PocketBase) error {

	// Get rollout
	rollout, err := app.Dao().FindRecordById("rollouts", e.Record.GetString("id"))
	if err != nil {
		return err
	}

	// Get project
	project, err := app.Dao().FindRecordById("projects", rollout.GetString("project"))
	if err != nil {
		return err
	}

	// Get deployment
	deployment, err := app.Dao().FindRecordById("deployments", rollout.GetString("deployment"))
	if err != nil {
		return err
	}

	// Get user
	user, err := app.Dao().FindRecordById("users", project.GetString("user"))
	if err != nil {
		return err
	}

	// If only the hide field was updated, do nothing and return
	if e.Record.GetBool("hide") != rollout.GetBool("hide") {
		if e.Record.GetString("endDate") != rollout.GetString("endDate") {
			// Continue with the rest of the code
		} else {
			return nil
		}
	}

	targetType := rollout.GetString("target_type")
	isCloud := isCloudTargetType(targetType)

	// Cloud-rollout update semantics for Phase 2.0:
	//
	//   - endDate set         → mark deployment_targets row as `deleting`
	//                           so the reconciler dispatches Provider.Destroy.
	//   - endDate just cleared → no-op for now. Re-arming a previously-ended
	//                           cloud rollout is not yet supported; the
	//                           original deployment_targets row was already
	//                           consumed by the destroy path. Operator must
	//                           create a fresh rollout. Logged loudly so
	//                           this isn't silent.
	//   - regular spec update → DEFERRED. Phase 2.0 does not yet propagate
	//                           image / env / scale changes to the cloud
	//                           target. Logged so the gap is not invisible.
	if isCloud {
		newEnd := e.Record.GetString("endDate")
		oldEnd := rollout.GetString("endDate")
		switch {
		case newEnd != "" && oldEnd == "":
			if err := markCloudTargetForDestroy(app, rollout.Id); err != nil {
				log.Printf("[CLOUD_DESTROY_MARK_FAIL] rollout=%s err=%v", rollout.Id, err)
				return err
			}
			log.Printf("[CLOUD_DESTROY_MARKED] rollout=%s", rollout.Id)
			return nil
		case newEnd == "" && oldEnd != "":
			log.Printf("[CLOUD_UPDATE_DEFERRED] rollout=%s reason=cloud_reactivation_not_supported", rollout.Id)
			e.Record.Set("startDate", time.Now().UTC().Format(time.RFC3339))
			return nil
		default:
			// Phase 2.1 minimal spec-update propagation: if the manifest
			// changed, flip every target row for this rollout back to
			// `pending` so the dispatcher re-deploys with the new spec
			// on the next reconcile tick. We refuse to flip if any
			// target has an in-flight operation — the dispatcher's
			// stale-spec check will catch the divergence and trigger a
			// fresh dispatch when the current op completes.
			oldManifest := rollout.GetString("manifest")
			newManifest := e.Record.GetString("manifest")
			if oldManifest != newManifest {
				if err := flipCloudTargetsToPendingOnRespec(app, rollout.Id); err != nil {
					log.Printf("[CLOUD_RESPEC_FAIL] rollout=%s err=%v", rollout.Id, err)
					return err
				}
				log.Printf("[CLOUD_RESPEC_REDISPATCH] rollout=%s", rollout.Id)
			} else {
				log.Printf("[CLOUD_UPDATE_NOOP] rollout=%s reason=no_spec_change", rollout.Id)
			}
			e.Record.Set("startDate", time.Now().UTC().Format(time.RFC3339))
			return nil
		}
	}

	// Check if endDate is set, if yes, delete rollout
	if e.Record.GetString("endDate") != "" {
		err = k8s.DeleteRollout(project.Id, deployment.Id)
		if err != nil {
			log.Println(err)
			return err
		}
		return nil
	} else if rollout.GetString("endDate") != "" {

		// Check if there is another rollout in the same project with no endDate
		running_rollout, err := app.Dao().FindFirstRecordByFilter("rollouts", "endDate = '' && project = {:project} && deployment = {:deployment}",
			dbx.Params{"project": rollout.GetString("project"), "deployment": rollout.GetString("deployment")},
		)
		if err != nil {
			// only throw error string if it doesn't contain "no rows"
			if contains := strings.Contains(err.Error(), "no rows"); !contains {
				return err
			}
		}

		// if there is another rollout in the same project with no endDate, set endDate to now on that rollout
		if running_rollout != nil && running_rollout.Id != rollout.Id {
			running_rollout.Set("endDate", time.Now().UTC().Format(time.RFC3339))
			err = app.Dao().SaveRecord(running_rollout)
			if err != nil {
				return err
			}
		}

		e.Record.Set("startDate", time.Now().UTC().Format(time.RFC3339))

		// If endDate was set before, but is not set anymore, create rollout again
		err = k8s.CreateOrUpdateRollout(rollout.Id, user, project.Id, deployment.Id, e.Record.GetString("manifest"))
		if err != nil {
			log.Println(err)
			return err
		}

		return nil

	} else {
		e.Record.Set("startDate", time.Now().UTC().Format(time.RFC3339))
		// Update rollout in k8s
		err = k8s.CreateOrUpdateRollout(rollout.Id, user, project.Id, deployment.Id, e.Record.GetString("manifest"))
		if err != nil {
			log.Println(err)
			return err
		}
	}

	return nil
}

func HandleRolloutDelete(e *core.RecordDeleteEvent, app *pocketbase.PocketBase) error {

	// Get rollout
	rollout, err := app.Dao().FindRecordById("rollouts", e.Record.GetString("id"))
	if err != nil {
		log.Println(err)
	}

	// Get project
	project, err := app.Dao().FindRecordById("projects", rollout.GetString("project"))
	if err != nil {
		log.Println(err)
	}

	// Get deployment
	deployment, err := app.Dao().FindRecordById("deployments", rollout.GetString("deployment"))
	if err != nil {
		log.Println(err)
	}

	// Phase 2.1 orphan defense: a cloud rollout with a live
	// deployment_targets row CANNOT be hard-deleted. The PocketBase
	// cascade would remove the AutoStack-side record while leaving the
	// Cloud Run service running, costing money and falling out of any
	// inventory. Returning an HTTP error here BLOCKS the delete (this
	// handler runs in OnRecordBeforeDeleteRequest); the operator must
	// end the rollout first, let the reconciler dispatch Destroy, and
	// THEN delete the rollout once the target row reaches `deleted`.
	if isCloudTargetType(rollout.GetString("target_type")) {
		live, err := app.Dao().FindRecordsByFilter(
			"deployment_targets",
			"rollout = {:rollout} && status != 'deleted'",
			"", 1, 0,
			map[string]interface{}{"rollout": rollout.Id},
		)
		if err == nil && len(live) > 0 {
			log.Printf("[CLOUD_DELETE_REFUSED] rollout=%s target=%s status=%s reason=live_target", rollout.Id, live[0].Id, live[0].GetString("status"))
			return echo.NewHTTPError(409,
				"cloud rollout has a live deployment_target. End the rollout (set endDate) "+
					"so the reconciler can destroy it, then delete the rollout after the target reaches status=deleted.")
		}
		// Either no target row, or all targets already deleted — safe to delete.
		log.Printf("[CLOUD_DELETE_ALLOWED] rollout=%s reason=no_live_targets", rollout.Id)
		return nil
	}

	// Check if endDate is set, if no, delete rollout
	if rollout.GetString("endDate") == "" {
		err = k8s.DeleteRollout(project.Id, deployment.Id)
		if err != nil {
			log.Println(err)
		}
	}

	return nil
}
