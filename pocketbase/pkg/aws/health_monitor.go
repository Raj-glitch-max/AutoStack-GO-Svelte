package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/health"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/pocketbase/pocketbase/core"
)

// HealthMonitor polls AWS services for resource health
type HealthMonitor struct {
	app         core.App
	credManager *CredentialManager
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(app core.App, credManager *CredentialManager) *HealthMonitor {
	return &HealthMonitor{
		app:         app,
		credManager: credManager,
	}
}

// GetDeploymentHealth aggregates health for all resources in a deployment
func (hm *HealthMonitor) GetDeploymentHealth(ctx context.Context, deploymentID string) (*health.DeploymentHealth, error) {
	deployment, err := hm.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to find deployment: %w", err)
	}

	outputs := make(map[string]interface{})
	if outputsStr := deployment.GetString("outputs"); outputsStr != "" {
		if err := json.Unmarshal([]byte(outputsStr), &outputs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal outputs: %w", err)
		}
	}

	cfg, err := hm.credManager.GetConfigForUser(ctx, deployment.GetString("user"))
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS config: %w", err)
	}

	res := &health.DeploymentHealth{
		DeploymentID:   deploymentID,
		DeploymentType: "aws",
		OverallStatus:  health.StatusHealthy,
		Resources:      make([]health.ResourceHealth, 0),
	}

	// Helper to get string value from Terraform output object
	getOutputValue := func(key string) (string, bool) {
		if obj, ok := outputs[key].(map[string]interface{}); ok {
			if val, ok := obj["value"].(string); ok {
				return val, true
			}
		}
		// Fallback for simple string outputs (if any)
		if val, ok := outputs[key].(string); ok {
			return val, true
		}
		return "", false
	}

	// 1. Check ECS Health
	if serviceName, ok := getOutputValue("ecs_service_name"); ok {
		clusterName, _ := getOutputValue("cluster_name")
		ecsHealth := hm.checkECSHealth(ctx, cfg, clusterName, serviceName)
		res.Resources = append(res.Resources, ecsHealth)
		if ecsHealth.Status != health.StatusHealthy {
			res.OverallStatus = ecsHealth.Status
		}
	}

	// 2. Check ALB Health
	if tgArn, ok := getOutputValue("target_group_arn"); ok {
		albHealth := hm.checkALBHealth(ctx, cfg, tgArn)
		res.Resources = append(res.Resources, albHealth)
		if albHealth.Status != health.StatusHealthy && res.OverallStatus == health.StatusHealthy {
			res.OverallStatus = albHealth.Status
		}
	}

	// 3. Check RDS Health
	if dbID, ok := getOutputValue("db_instance_id"); ok {
		rdsHealth := hm.checkRDSHealth(ctx, cfg, dbID)
		res.Resources = append(res.Resources, rdsHealth)
		if rdsHealth.Status != health.StatusHealthy && res.OverallStatus == health.StatusHealthy {
			res.OverallStatus = rdsHealth.Status
		}
	}

	// 4. Check Lambda Health
	if lambdaName, ok := getOutputValue("lambda_function_name"); ok {
		lambdaHealth := hm.checkLambdaHealth(ctx, cfg, lambdaName)
		res.Resources = append(res.Resources, lambdaHealth)
		if lambdaHealth.Status != health.StatusHealthy && res.OverallStatus == health.StatusHealthy {
			res.OverallStatus = lambdaHealth.Status
		}
	}

	return res, nil
}

func (hm *HealthMonitor) checkECSHealth(ctx context.Context, cfg aws.Config, cluster, service string) health.ResourceHealth {
	client := ecs.NewFromConfig(cfg)
	
	resp, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(cluster),
		Services: []string{service},
	})
	
	if err != nil {
		return health.ResourceHealth{
			ResourceID:   service,
			ResourceType: "ECS_SERVICE",
			Status:       health.StatusUnknown,
			Message:      fmt.Sprintf("Failed to describe service: %v", err),
		}
	}

	if len(resp.Services) == 0 {
		return health.ResourceHealth{
			ResourceID:   service,
			ResourceType: "ECS_SERVICE",
			Status:       health.StatusUnhealthy,
			Message:      "Service not found",
		}
	}

	svc := resp.Services[0]
	status := health.StatusHealthy
	if svc.RunningCount < svc.DesiredCount {
		status = health.StatusUnhealthy
	}

	return health.ResourceHealth{
		ResourceID:   service,
		ResourceType: "ECS_SERVICE",
		Status:       status,
		Message:      fmt.Sprintf("Status: %s, Running: %d/%d", aws.ToString(svc.Status), svc.RunningCount, svc.DesiredCount),
		Details:      svc,
	}
}

func (hm *HealthMonitor) checkALBHealth(ctx context.Context, cfg aws.Config, tgArn string) health.ResourceHealth {
	client := elbv2.NewFromConfig(cfg)
	
	resp, err := client.DescribeTargetHealth(ctx, &elbv2.DescribeTargetHealthInput{
		TargetGroupArn: aws.String(tgArn),
	})
	
	if err != nil {
		return health.ResourceHealth{
			ResourceID:   tgArn,
			ResourceType: "ALB_TARGET_GROUP",
			Status:       health.StatusUnknown,
			Message:      fmt.Sprintf("Failed to describe target health: %v", err),
		}
	}

	healthyCount := 0
	totalCount := len(resp.TargetHealthDescriptions)
	
	for _, desc := range resp.TargetHealthDescriptions {
		if desc.TargetHealth.State == "healthy" {
			healthyCount++
		}
	}

	status := health.StatusHealthy
	if healthyCount == 0 && totalCount > 0 {
		status = health.StatusUnhealthy
	} else if healthyCount < totalCount {
		status = health.StatusPending // Some targets are down or draining
	}

	return health.ResourceHealth{
		ResourceID:   tgArn,
		ResourceType: "ALB_TARGET_GROUP",
		Status:       status,
		Message:      fmt.Sprintf("Healthy: %d/%d targets", healthyCount, totalCount),
		Details:      resp.TargetHealthDescriptions,
	}
}

func (hm *HealthMonitor) checkRDSHealth(ctx context.Context, cfg aws.Config, dbID string) health.ResourceHealth {
	client := rds.NewFromConfig(cfg)
	
	resp, err := client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(dbID),
	})
	
	if err != nil {
		return health.ResourceHealth{
			ResourceID:   dbID,
			ResourceType: "RDS_INSTANCE",
			Status:       health.StatusUnknown,
			Message:      fmt.Sprintf("Failed to describe DB instance: %v", err),
		}
	}

	if len(resp.DBInstances) == 0 {
		return health.ResourceHealth{
			ResourceID:   dbID,
			ResourceType: "RDS_INSTANCE",
			Status:       health.StatusUnhealthy,
			Message:      "Database instance not found",
		}
	}

	db := resp.DBInstances[0]
	status := health.StatusHealthy
	if aws.ToString(db.DBInstanceStatus) != "available" {
		status = health.StatusUnhealthy
	}

	return health.ResourceHealth{
		ResourceID:   dbID,
		ResourceType: "RDS_INSTANCE",
		Status:       status,
		Message:      fmt.Sprintf("Status: %s", aws.ToString(db.DBInstanceStatus)),
		Details:      db,
	}
}

func (hm *HealthMonitor) checkLambdaHealth(ctx context.Context, cfg aws.Config, functionName string) health.ResourceHealth {
	client := lambda.NewFromConfig(cfg)
	
	resp, err := client.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(functionName),
	})
	
	if err != nil {
		return health.ResourceHealth{
			ResourceID:   functionName,
			ResourceType: "LAMBDA_FUNCTION",
			Status:       health.StatusUnknown,
			Message:      fmt.Sprintf("Failed to get function: %v", err),
		}
	}

	status := health.StatusHealthy
	state := resp.Configuration.State
	if state != "Active" {
		status = health.StatusPending
	}

	return health.ResourceHealth{
		ResourceID:   functionName,
		ResourceType: "LAMBDA_FUNCTION",
		Status:       status,
		Message:      fmt.Sprintf("State: %s, Last Update: %s", state, string(resp.Configuration.LastUpdateStatus)),
		Details:      resp.Configuration,
	}
}
