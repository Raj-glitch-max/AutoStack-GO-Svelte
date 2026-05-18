package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/janlauber/one-click/pkg/providers"
	"github.com/janlauber/one-click/pkg/secrets"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
)

// Cloud deployment HTTP surface.  Calls into the EXISTING pkg/providers
// implementations (real AWS SDK v2 / Google Cloud Run SDK code) — this
// package adds no provider logic, only request validation, persistence,
// and operator-friendly response shaping.
//
// HONEST SCOPE BOUNDARY: the provider Deploy() implementations are real
// code paths.  Whether they SUCCEED for any given request depends entirely
// on the cloud_accounts row's credentials.  In an environment with valid
// AWS / GCP credentials this writes real cloud resources; without them
// the call returns the actual provider error (e.g. InvalidClientTokenId)
// — never a fake success.

// HandleCloudDeploymentCreate
// POST /api/v1/cloud-deployments
//
// Body:
//   {
//     "cloud_account_id": "<id of a cloud_accounts row>",
//     "name":             "string",
//     "image":            "registry/repo:tag",
//     "cpu_vcpu":         number,
//     "memory_mb":        number,
//     "replicas":         number,
//     "container_port":   number
//   }
func HandleCloudDeploymentCreate(c echo.Context, app *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}

	var req struct {
		CloudAccountID string  `json:"cloud_account_id"`
		Name           string  `json:"name"`
		Image          string  `json:"image"`
		CPU            float64 `json:"cpu_vcpu"`
		MemoryMB       int     `json:"memory_mb"`
		Replicas       int     `json:"replicas"`
		ContainerPort  int     `json:"container_port"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request: " + err.Error()})
	}
	if req.Name == "" || req.Image == "" || req.CloudAccountID == "" {
		return c.JSON(400, map[string]string{"error": "name, image, and cloud_account_id are required"})
	}
	if req.CPU <= 0 {
		req.CPU = 1.0
	}
	if req.MemoryMB <= 0 {
		req.MemoryMB = 512
	}
	if req.Replicas <= 0 {
		req.Replicas = 1
	}

	acctRec, err := app.Dao().FindRecordById("cloud_accounts", req.CloudAccountID)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "cloud_account not found"})
	}
	if acctRec.GetString("user") != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}
	if acctRec.GetString("status") != "active" {
		return c.JSON(400, map[string]string{
			"error": fmt.Sprintf("cloud_account is not active (status=%q). Re-validate it first.",
				acctRec.GetString("status")),
		})
	}

	provName, err := providerNameFor(acctRec.GetString("provider"))
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	prov, err := providers.GetProvider(provName)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "provider not available: " + err.Error()})
	}

	credentials, err := secrets.Decrypt(acctRec.GetString("credentials_encrypted"))
	if err != nil {
		return c.JSON(500, map[string]string{"error": "credential decryption failed"})
	}

	account := &providers.CloudAccount{
		ID:                   acctRec.Id,
		Name:                 acctRec.GetString("name"),
		Provider:             acctRec.GetString("provider"),
		Region:               acctRec.GetString("region"),
		CredentialsEncrypted: credentials,
	}

	// Persist row in "pending" BEFORE calling the provider, so any provider
	// failure is recorded on the deployment row for operator visibility.
	depRec, err := createCloudDeploymentRecord(app, user.Id, acctRec, req)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "save deployment: " + err.Error()})
	}

	// Build the provider DeploySpec.
	spec := buildDeploySpec(depRec.Id, req)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, deployErr := prov.Deploy(ctx, account, spec)

	if deployErr != nil {
		depRec.Set("status", "error")
		depRec.Set("last_error", truncateErr(deployErr.Error(), 2000))
		_ = app.Dao().SaveRecord(depRec)
		return c.JSON(502, map[string]interface{}{
			"id":     depRec.Id,
			"status": "error",
			"error":  deployErr.Error(),
		})
	}

	depRec.Set("status", "provisioning")
	if result.ExternalID != "" {
		depRec.Set("external_id", result.ExternalID)
	}
	if result.EndpointURL != "" {
		depRec.Set("endpoint_url", result.EndpointURL)
	}
	if result.OperationID != "" {
		depRec.Set("operation_id", result.OperationID)
	}
	depRec.Set("deployed_at", time.Now().UTC().Format(time.RFC3339))
	depRec.Set("last_error", "")
	if err := app.Dao().SaveRecord(depRec); err != nil {
		return c.JSON(500, map[string]string{"error": "persist post-deploy state: " + err.Error()})
	}

	return c.JSON(201, map[string]interface{}{
		"id":           depRec.Id,
		"status":       "provisioning",
		"external_id":  result.ExternalID,
		"endpoint_url": result.EndpointURL,
		"message":      result.Message,
	})
}

// HandleCloudDeploymentList
// GET /api/v1/cloud-deployments
func HandleCloudDeploymentList(c echo.Context, app *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	recs, err := app.Dao().FindRecordsByFilter("cloud_deployments",
		"user = {:u}", "-created", 100, 0,
		map[string]interface{}{"u": user.Id})
	if err != nil {
		// Empty-result on PB sometimes surfaces as error; coerce to empty list.
		return c.JSON(200, []interface{}{})
	}
	out := make([]map[string]interface{}, 0, len(recs))
	for _, r := range recs {
		out = append(out, cloudDepView(r))
	}
	return c.JSON(200, out)
}

// HandleCloudDeploymentGet
// GET /api/v1/cloud-deployments/:id
func HandleCloudDeploymentGet(c echo.Context, app *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	rec, err := app.Dao().FindRecordById("cloud_deployments", c.PathParam("id"))
	if err != nil {
		return c.JSON(404, map[string]string{"error": "not found"})
	}
	if rec.GetString("user") != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}
	return c.JSON(200, cloudDepView(rec))
}

// HandleCloudDeploymentDestroy
// DELETE /api/v1/cloud-deployments/:id
func HandleCloudDeploymentDestroy(c echo.Context, app *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}
	rec, err := app.Dao().FindRecordById("cloud_deployments", c.PathParam("id"))
	if err != nil {
		return c.JSON(404, map[string]string{"error": "not found"})
	}
	if rec.GetString("user") != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}
	// Fetch the cloud account and call the provider's Destroy.
	acctRec, err := app.Dao().FindRecordById("cloud_accounts", rec.GetString("account"))
	if err != nil {
		return c.JSON(500, map[string]string{"error": "cloud_account lookup failed"})
	}
	provName, err := providerNameFor(acctRec.GetString("provider"))
	if err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}
	prov, err := providers.GetProvider(provName)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "provider not available"})
	}
	credentials, err := secrets.Decrypt(acctRec.GetString("credentials_encrypted"))
	if err != nil {
		return c.JSON(500, map[string]string{"error": "credential decryption failed"})
	}
	account := &providers.CloudAccount{
		ID:                   acctRec.Id,
		Provider:             acctRec.GetString("provider"),
		Region:               acctRec.GetString("region"),
		CredentialsEncrypted: credentials,
	}
	target := &providers.DeploymentTarget{
		ID:         rec.Id,
		ExternalID: rec.GetString("external_id"),
		Region:     acctRec.GetString("region"),
		Provider:   acctRec.GetString("provider"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := prov.Destroy(ctx, account, target); err != nil {
		rec.Set("last_error", truncateErr(err.Error(), 2000))
		_ = app.Dao().SaveRecord(rec)
		return c.JSON(502, map[string]string{"error": err.Error()})
	}
	rec.Set("status", "destroyed")
	rec.Set("last_error", "")
	_ = app.Dao().SaveRecord(rec)
	return c.JSON(200, map[string]string{"status": "destroyed"})
}

// providerNameFor maps the cloud_accounts.provider string ("aws"/"gcp"/"azure")
// to the providers package's registered name.
func providerNameFor(s string) (string, error) {
	switch s {
	case "aws":
		return providers.ProviderAWSECS, nil
	case "gcp":
		return providers.ProviderGCPCloudRun, nil
	case "azure":
		return providers.ProviderAzureACA, nil
	default:
		return "", fmt.Errorf("unsupported provider %q", s)
	}
}

func createCloudDeploymentRecord(app *pocketbase.PocketBase, userID string, acctRec *models.Record, req struct {
	CloudAccountID string  `json:"cloud_account_id"`
	Name           string  `json:"name"`
	Image          string  `json:"image"`
	CPU            float64 `json:"cpu_vcpu"`
	MemoryMB       int     `json:"memory_mb"`
	Replicas       int     `json:"replicas"`
	ContainerPort  int     `json:"container_port"`
}) (*models.Record, error) {
	coll, err := app.Dao().FindCollectionByNameOrId("cloud_deployments")
	if err != nil {
		return nil, err
	}
	rec := models.NewRecord(coll)
	rec.Set("name", req.Name)
	rec.Set("user", userID)
	rec.Set("account", acctRec.Id)
	rec.Set("provider", acctRec.GetString("provider"))
	rec.Set("region", acctRec.GetString("region"))
	rec.Set("image", req.Image)
	rec.Set("cpu_vcpu", req.CPU)
	rec.Set("memory_mb", req.MemoryMB)
	rec.Set("replicas", req.Replicas)
	rec.Set("status", "pending")
	return rec, app.Dao().SaveRecord(rec)
}

func buildDeploySpec(rolloutID string, req struct {
	CloudAccountID string  `json:"cloud_account_id"`
	Name           string  `json:"name"`
	Image          string  `json:"image"`
	CPU            float64 `json:"cpu_vcpu"`
	MemoryMB       int     `json:"memory_mb"`
	Replicas       int     `json:"replicas"`
	ContainerPort  int     `json:"container_port"`
}) *providers.DeploySpec {
	repo, tag := req.Image, "latest"
	if idx := indexLastByte(req.Image, ':'); idx > 0 {
		repo = req.Image[:idx]
		tag = req.Image[idx+1:]
	}
	port := req.ContainerPort
	if port == 0 {
		port = 80
	}
	return &providers.DeploySpec{
		RolloutID: rolloutID,
		Image: providers.ImageSpec{
			Repository: repo,
			Tag:        tag,
			PullPolicy: "IfNotPresent",
		},
		Compute: providers.ComputeSpec{
			CPURequestVCPU:  req.CPU,
			CPULimitVCPU:    req.CPU,
			MemoryRequestMB: req.MemoryMB,
			MemoryLimitMB:   req.MemoryMB,
		},
		Scale: providers.ScaleSpec{
			MinReplicas: req.Replicas,
			MaxReplicas: req.Replicas,
		},
		Network: providers.NetworkSpec{
			Interfaces: []providers.NetworkInterface{{
				ContainerPort: port,
				Protocol:      "tcp",
			}},
		},
	}
}

func cloudDepView(r *models.Record) map[string]interface{} {
	return map[string]interface{}{
		"id":           r.Id,
		"name":         r.GetString("name"),
		"account":      r.GetString("account"),
		"provider":     r.GetString("provider"),
		"region":       r.GetString("region"),
		"image":        r.GetString("image"),
		"cpu_vcpu":     r.GetFloat("cpu_vcpu"),
		"memory_mb":    r.GetInt("memory_mb"),
		"replicas":     r.GetInt("replicas"),
		"status":       r.GetString("status"),
		"external_id":  r.GetString("external_id"),
		"endpoint_url": r.GetString("endpoint_url"),
		"operation_id": r.GetString("operation_id"),
		"last_error":   r.GetString("last_error"),
		"deployed_at":  r.GetString("deployed_at"),
		"created":      r.GetCreated().Time().UTC().Format(time.RFC3339),
		"updated":      r.GetUpdated().Time().UTC().Format(time.RFC3339),
	}
}

func indexLastByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func truncateErr(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
