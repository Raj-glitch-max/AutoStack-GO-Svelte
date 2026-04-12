package validation

import (
	"testing"
)

// TestValidationIntegration tests the complete validation flow
func TestValidationIntegration(t *testing.T) {
	t.Run("Complete valid request", func(t *testing.T) {
		blueprint := "web-application"
		region := "us-east-1"
		variables := map[string]interface{}{
			"vcpu":                0.25,
			"memory":              0.5,
			"task_count":          1,
			"db_instance_class":   "db.t3.micro",
			"db_storage_gb":       20,
			"db_engine":           "postgres",
			"lcu_hours":           730.0,
			"nat_data_gb":         50.0,
			"cloudwatch_logs_gb":  5.0,
		}

		err := ValidateCostEstimateRequest(blueprint, region, variables)
		if err != nil {
			t.Errorf("Expected no error for valid request, got: %v", err)
		}
	})

	t.Run("Invalid blueprint in complete request", func(t *testing.T) {
		blueprint := "invalid-blueprint"
		region := "us-east-1"
		variables := map[string]interface{}{}

		err := ValidateCostEstimateRequest(blueprint, region, variables)
		if err == nil {
			t.Error("Expected error for invalid blueprint, got nil")
		}
	})

	t.Run("Invalid region in complete request", func(t *testing.T) {
		blueprint := "static-website"
		region := "invalid-region"
		variables := map[string]interface{}{}

		err := ValidateCostEstimateRequest(blueprint, region, variables)
		if err == nil {
			t.Error("Expected error for invalid region, got nil")
		}
	})

	t.Run("Invalid configuration in complete request", func(t *testing.T) {
		blueprint := "web-application"
		region := "us-east-1"
		variables := map[string]interface{}{
			"vcpu": 99.0, // Invalid vCPU value
		}

		err := ValidateCostEstimateRequest(blueprint, region, variables)
		if err == nil {
			t.Error("Expected error for invalid configuration, got nil")
		}
	})
}

// TestAllBlueprintTypes ensures all blueprint types can be validated
func TestAllBlueprintTypes(t *testing.T) {
	blueprints := []string{
		"static-website",
		"web-application",
		"full-stack-app",
		"microservices",
	}

	region := "us-east-1"

	for _, blueprint := range blueprints {
		t.Run(blueprint, func(t *testing.T) {
			err := ValidateCostEstimateRequest(blueprint, region, nil)
			if err != nil {
				t.Errorf("Blueprint %s should be valid with nil variables, got error: %v", blueprint, err)
			}
		})
	}
}

// TestAllAWSRegions ensures all AWS regions can be validated
func TestAllAWSRegions(t *testing.T) {
	regions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1",
	}

	blueprint := "static-website"

	for _, region := range regions {
		t.Run(region, func(t *testing.T) {
			err := ValidateCostEstimateRequest(blueprint, region, nil)
			if err != nil {
				t.Errorf("Region %s should be valid, got error: %v", region, err)
			}
		})
	}
}

// TestEdgeCases tests edge cases in validation
func TestEdgeCases(t *testing.T) {
	t.Run("Empty variables map", func(t *testing.T) {
		err := ValidateCostEstimateRequest("static-website", "us-east-1", map[string]interface{}{})
		if err != nil {
			t.Errorf("Empty variables map should be valid, got error: %v", err)
		}
	})

	t.Run("Nil variables", func(t *testing.T) {
		err := ValidateCostEstimateRequest("static-website", "us-east-1", nil)
		if err != nil {
			t.Errorf("Nil variables should be valid, got error: %v", err)
		}
	})

	t.Run("Extra unknown variables", func(t *testing.T) {
		variables := map[string]interface{}{
			"storage_gb":      10.0,
			"unknown_field":   "should be ignored",
			"another_unknown": 123,
		}
		err := ValidateCostEstimateRequest("static-website", "us-east-1", variables)
		if err != nil {
			t.Errorf("Unknown variables should be ignored, got error: %v", err)
		}
	})

	t.Run("Zero values", func(t *testing.T) {
		variables := map[string]interface{}{
			"storage_gb":         0.0,
			"requests_per_month": 0,
		}
		err := ValidateCostEstimateRequest("static-website", "us-east-1", variables)
		if err != nil {
			t.Errorf("Zero values should be valid, got error: %v", err)
		}
	})
}
