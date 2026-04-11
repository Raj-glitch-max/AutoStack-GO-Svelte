package models

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tests"
)

func TestAwsPricingCacheSchema(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection, err := app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		t.Fatalf("Failed to find awsPricingCache collection: %v", err)
	}

	// Test required fields
	requiredFields := []string{"region", "service", "productFamily", "pricePerHour", "pricePerMonth", "currency", "fetchedAt", "validUntil"}
	for _, fieldName := range requiredFields {
		field := collection.Schema.GetFieldByName(fieldName)
		if field == nil {
			t.Errorf("Required field %s not found in awsPricingCache schema", fieldName)
		}
		if !field.Required {
			t.Errorf("Field %s should be required in awsPricingCache schema", fieldName)
		}
	}

	// Test optional fields
	optionalFields := []string{"instanceType", "vcpu", "memory"}
	for _, fieldName := range optionalFields {
		field := collection.Schema.GetFieldByName(fieldName)
		if field == nil {
			t.Errorf("Optional field %s not found in awsPricingCache schema", fieldName)
		}
		if field.Required {
			t.Errorf("Field %s should be optional in awsPricingCache schema", fieldName)
		}
	}

	// Test field types
	regionField := collection.Schema.GetFieldByName("region")
	if regionField.Type != "text" {
		t.Errorf("Region field should be text type, got %s", regionField.Type)
	}

	priceField := collection.Schema.GetFieldByName("pricePerHour")
	if priceField.Type != "number" {
		t.Errorf("PricePerHour field should be number type, got %s", priceField.Type)
	}
}

func TestCostEstimatesSchema(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection, err := app.Dao().FindCollectionByNameOrId("costEstimates")
	if err != nil {
		t.Fatalf("Failed to find costEstimates collection: %v", err)
	}

	// Test required fields
	requiredFields := []string{"deployment", "blueprint", "region", "computeMonthly", "networkingMonthly", "storageMonthly", "transferMonthly", "totalEstimate", "rangeMin", "rangeMax", "pricingVersion"}
	for _, fieldName := range requiredFields {
		field := collection.Schema.GetFieldByName(fieldName)
		if field == nil {
			t.Errorf("Required field %s not found in costEstimates schema", fieldName)
		}
		if !field.Required {
			t.Errorf("Field %s should be required in costEstimates schema", fieldName)
		}
	}

	// Test deployment relation
	deploymentField := collection.Schema.GetFieldByName("deployment")
	if deploymentField.Type != "relation" {
		t.Errorf("Deployment field should be relation type, got %s", deploymentField.Type)
	}

	// Test cost fields are numbers
	costFields := []string{"computeMonthly", "networkingMonthly", "storageMonthly", "transferMonthly", "totalEstimate", "rangeMin", "rangeMax"}
	for _, fieldName := range costFields {
		field := collection.Schema.GetFieldByName(fieldName)
		if field.Type != "number" {
			t.Errorf("Field %s should be number type, got %s", fieldName, field.Type)
		}
	}
}

func TestActualCostsSchema(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

	// Test required fields
	requiredFields := []string{"deployment", "costToDate", "projectedMonthly", "variance", "breakdown", "periodStart", "periodEnd", "fetchedAt"}
	for _, fieldName := range requiredFields {
		field := collection.Schema.GetFieldByName(fieldName)
		if field == nil {
			t.Errorf("Required field %s not found in actualCosts schema", fieldName)
		}
		if !field.Required {
			t.Errorf("Field %s should be required in actualCosts schema", fieldName)
		}
	}

	// Test deployment relation
	deploymentField := collection.Schema.GetFieldByName("deployment")
	if deploymentField.Type != "relation" {
		t.Errorf("Deployment field should be relation type, got %s", deploymentField.Type)
	}

	// Test breakdown is JSON
	breakdownField := collection.Schema.GetFieldByName("breakdown")
	if breakdownField.Type != "json" {
		t.Errorf("Breakdown field should be json type, got %s", breakdownField.Type)
	}

	// Test date fields
	dateFields := []string{"periodStart", "periodEnd", "fetchedAt"}
	for _, fieldName := range dateFields {
		field := collection.Schema.GetFieldByName(fieldName)
		if field.Type != "date" {
			t.Errorf("Field %s should be date type, got %s", fieldName, field.Type)
		}
	}
}

func TestCostAlertsSchema(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection, err := app.Dao().FindCollectionByNameOrId("costAlerts")
	if err != nil {
		t.Fatalf("Failed to find costAlerts collection: %v", err)
	}

	// Test required fields
	requiredFields := []string{"deployment", "user", "type", "threshold", "triggered", "actualCost", "estimatedCost", "variance", "message", "acknowledged"}
	for _, fieldName := range requiredFields {
		field := collection.Schema.GetFieldByName(fieldName)
		if field == nil {
			t.Errorf("Required field %s not found in costAlerts schema", fieldName)
		}
		if !field.Required {
			t.Errorf("Field %s should be required in costAlerts schema", fieldName)
		}
	}

	// Test relations
	deploymentField := collection.Schema.GetFieldByName("deployment")
	if deploymentField.Type != "relation" {
		t.Errorf("Deployment field should be relation type, got %s", deploymentField.Type)
	}

	userField := collection.Schema.GetFieldByName("user")
	if userField.Type != "relation" {
		t.Errorf("User field should be relation type, got %s", userField.Type)
	}

	// Test select field
	typeField := collection.Schema.GetFieldByName("type")
	if typeField.Type != "select" {
		t.Errorf("Type field should be select type, got %s", typeField.Type)
	}

	// Test boolean fields
	boolFields := []string{"triggered", "acknowledged"}
	for _, fieldName := range boolFields {
		field := collection.Schema.GetFieldByName(fieldName)
		if field.Type != "bool" {
			t.Errorf("Field %s should be bool type, got %s", fieldName, field.Type)
		}
	}
}

func TestCostEstimateValidation(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection, err := app.Dao().FindCollectionByNameOrId("costEstimates")
	if err != nil {
		t.Fatalf("Failed to find costEstimates collection: %v", err)
	}

	// Test valid cost estimate record
	record := models.NewRecord(collection)
	record.Set("deployment", "test_deployment_id")
	record.Set("blueprint", "static-website")
	record.Set("region", "us-east-1")
	record.Set("computeMonthly", 29.15)
	record.Set("networkingMonthly", 16.20)
	record.Set("storageMonthly", 2.30)
	record.Set("transferMonthly", 9.00)
	record.Set("totalEstimate", 56.65)
	record.Set("rangeMin", 45.32)
	record.Set("rangeMax", 79.31)
	record.Set("pricingVersion", "2026-04-11")

	if err := app.Dao().ValidateAndFillRecord(record, nil); err != nil {
		t.Errorf("Valid cost estimate record should pass validation: %v", err)
	}

	// Test range consistency (min <= total <= max)
	if record.GetFloat("rangeMin") > record.GetFloat("totalEstimate") {
		t.Error("Range min should be <= total estimate")
	}
	if record.GetFloat("totalEstimate") > record.GetFloat("rangeMax") {
		t.Error("Total estimate should be <= range max")
	}

	// Test positive costs
	costFields := []string{"computeMonthly", "networkingMonthly", "storageMonthly", "transferMonthly", "totalEstimate", "rangeMin", "rangeMax"}
	for _, fieldName := range costFields {
		if record.GetFloat(fieldName) < 0 {
			t.Errorf("Cost field %s should be positive, got %f", fieldName, record.GetFloat(fieldName))
		}
	}
}

func TestActualCostValidation(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

	// Test valid actual cost record
	record := models.NewRecord(collection)
	record.Set("deployment", "test_deployment_id")
	record.Set("costToDate", 47.23)
	record.Set("projectedMonthly", 62.18)
	record.Set("variance", 9.76)
	record.Set("breakdown", map[string]interface{}{
		"AmazonS3":        2.45,
		"AmazonCloudFront": 8.90,
		"AmazonRoute53":   0.50,
	})
	record.Set("periodStart", time.Now().AddDate(0, 0, -10))
	record.Set("periodEnd", time.Now())
	record.Set("fetchedAt", time.Now())

	if err := app.Dao().ValidateAndFillRecord(record, nil); err != nil {
		t.Errorf("Valid actual cost record should pass validation: %v", err)
	}

	// Test positive costs
	if record.GetFloat("costToDate") < 0 {
		t.Error("Cost to date should be positive")
	}
	if record.GetFloat("projectedMonthly") < 0 {
		t.Error("Projected monthly should be positive")
	}

	// Test period consistency
	periodStart := record.GetDateTime("periodStart")
	periodEnd := record.GetDateTime("periodEnd")
	if periodStart.After(periodEnd.Time()) {
		t.Error("Period start should be before period end")
	}
}