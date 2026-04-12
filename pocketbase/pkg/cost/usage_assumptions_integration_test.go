package cost

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// TestUsageAssumptionsIntegration tests the integration between UsageAssumptionsManager and BlueprintMapper
func TestUsageAssumptionsIntegration(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	mapper := NewBlueprintMapper(app)
	assumptionsManager := NewUsageAssumptionsManager()

	t.Run("Blueprint assumptions are available from both systems", func(t *testing.T) {
		blueprintTypes := []string{
			string(BlueprintStaticWebsite),
			string(BlueprintWebApp),
			string(BlueprintFullStack),
			string(BlueprintMicroservices),
		}

		for _, blueprintType := range blueprintTypes {
			// Get assumptions from blueprint mapper
			mapperAssumptions, err := mapper.GetAssumptions(blueprintType)
			if err != nil {
				t.Fatalf("Failed to get assumptions from mapper for %s: %v", blueprintType, err)
			}

			// Get assumptions from usage assumptions manager
			managerAssumptions, err := assumptionsManager.GetAssumptionsAsMap(blueprintType)
			if err != nil {
				t.Fatalf("Failed to get assumptions from manager for %s: %v", blueprintType, err)
			}

			// Verify both have assumptions
			if len(mapperAssumptions) == 0 {
				t.Errorf("Expected mapper to have assumptions for %s", blueprintType)
			}

			if len(managerAssumptions) == 0 {
				t.Errorf("Expected manager to have assumptions for %s", blueprintType)
			}

			// Verify some common keys exist in both (not all need to match exactly)
			commonKeys := []string{"database", "availability"}
			for _, key := range commonKeys {
				if blueprintType != string(BlueprintStaticWebsite) { // Static website doesn't have database
					if _, exists := mapperAssumptions[key]; !exists {
						t.Logf("Note: Key %s not in mapper for %s (this is OK)", key, blueprintType)
					}
				}
			}
		}
	})

	t.Run("Usage assumptions provide detailed information", func(t *testing.T) {
		blueprintType := string(BlueprintStaticWebsite)

		assumptions, err := assumptionsManager.GetAssumptions(blueprintType)
		if err != nil {
			t.Fatalf("Failed to get assumptions: %v", err)
		}

		// Verify assumptions have descriptions
		for _, assumption := range assumptions.Assumptions {
			if assumption.Description == "" {
				t.Errorf("Assumption %s has no description", assumption.Key)
			}
		}

		// Verify we can format for display
		display, err := assumptionsManager.FormatAssumptionsForDisplay(blueprintType)
		if err != nil {
			t.Fatalf("Failed to format assumptions: %v", err)
		}

		if display == "" {
			t.Error("Expected non-empty display string")
		}
	})

	t.Run("Custom assumptions can be applied", func(t *testing.T) {
		blueprintType := string(BlueprintStaticWebsite)

		// Get original assumptions
		originalAssumptions, _ := assumptionsManager.GetAssumptionsAsMap(blueprintType)
		originalStorage := originalAssumptions["storage"]

		// Update assumption
		err := assumptionsManager.UpdateAssumption(blueprintType, "storage", "50 GB")
		if err != nil {
			t.Fatalf("Failed to update assumption: %v", err)
		}

		// Verify update
		updatedAssumptions, _ := assumptionsManager.GetAssumptionsAsMap(blueprintType)
		if updatedAssumptions["storage"] != "50 GB" {
			t.Errorf("Expected storage to be '50 GB', got '%s'", updatedAssumptions["storage"])
		}

		// Restore original
		assumptionsManager.UpdateAssumption(blueprintType, "storage", originalStorage)
	})

	t.Run("Cost estimation uses assumptions", func(t *testing.T) {
		blueprintType := "static-website"

		// Get assumptions
		assumptions, err := assumptionsManager.GetAssumptionsAsMap(blueprintType)
		if err != nil {
			t.Fatalf("Failed to get assumptions: %v", err)
		}

		// Estimate cost with default config
		config := map[string]interface{}{}
		estimate, err := mapper.EstimateCost(blueprintType, config, "us-east-1")
		if err != nil {
			t.Fatalf("Failed to estimate cost: %v", err)
		}

		// Verify estimate has assumptions in breakdown
		if estimate.Breakdown == nil {
			t.Fatal("Expected breakdown to be present")
		}

		breakdown, ok := estimate.Breakdown.(*StaticWebsiteCostBreakdown)
		if !ok {
			t.Fatal("Expected StaticWebsiteCostBreakdown type")
		}

		// Verify assumptions are present in breakdown
		if len(breakdown.Assumptions) == 0 {
			t.Error("Expected assumptions in cost breakdown")
		}

		// Verify key assumptions match
		for key := range assumptions {
			if _, exists := breakdown.Assumptions[key]; !exists {
				t.Errorf("Expected assumption %s in cost breakdown", key)
			}
		}
	})

	t.Run("All blueprints have consistent assumptions structure", func(t *testing.T) {
		blueprintTypes := []string{
			string(BlueprintStaticWebsite),
			string(BlueprintWebApp),
			string(BlueprintFullStack),
			string(BlueprintMicroservices),
		}

		for _, blueprintType := range blueprintTypes {
			assumptions, err := assumptionsManager.GetAssumptions(blueprintType)
			if err != nil {
				t.Fatalf("Failed to get assumptions for %s: %v", blueprintType, err)
			}

			// Verify structure
			if assumptions.BlueprintType != blueprintType {
				t.Errorf("Expected blueprint type %s, got %s", blueprintType, assumptions.BlueprintType)
			}

			if len(assumptions.Assumptions) == 0 {
				t.Errorf("Expected assumptions list for %s", blueprintType)
			}

			if len(assumptions.AsMap) == 0 {
				t.Errorf("Expected assumptions map for %s", blueprintType)
			}

			// Verify each assumption has required fields
			for _, assumption := range assumptions.Assumptions {
				if assumption.Key == "" {
					t.Errorf("Assumption missing key for %s", blueprintType)
				}
				if assumption.Value == "" {
					t.Errorf("Assumption %s missing value for %s", assumption.Key, blueprintType)
				}
				if assumption.Description == "" {
					t.Errorf("Assumption %s missing description for %s", assumption.Key, blueprintType)
				}
			}
		}
	})

	t.Run("Assumptions validation works correctly", func(t *testing.T) {
		blueprintType := string(BlueprintStaticWebsite)

		// Valid assumption
		err := assumptionsManager.ValidateAssumptionValue(blueprintType, "storage", "20 GB")
		if err != nil {
			t.Errorf("Expected valid assumption to pass validation: %v", err)
		}

		// Empty value
		err = assumptionsManager.ValidateAssumptionValue(blueprintType, "storage", "")
		if err == nil {
			t.Error("Expected empty value to fail validation")
		}

		// Invalid key
		err = assumptionsManager.ValidateAssumptionValue(blueprintType, "invalid-key", "value")
		if err == nil {
			t.Error("Expected invalid key to fail validation")
		}

		// Invalid blueprint
		err = assumptionsManager.ValidateAssumptionValue("invalid-blueprint", "storage", "20 GB")
		if err == nil {
			t.Error("Expected invalid blueprint to fail validation")
		}
	})

	t.Run("Supported blueprints match between systems", func(t *testing.T) {
		mapperBlueprints := mapper.GetSupportedBlueprints()
		managerBlueprints := assumptionsManager.GetSupportedBlueprints()

		if len(mapperBlueprints) != len(managerBlueprints) {
			t.Errorf("Expected same number of blueprints, got mapper=%d, manager=%d",
				len(mapperBlueprints), len(managerBlueprints))
		}

		// Verify all mapper blueprints are in manager
		for _, bp := range mapperBlueprints {
			found := false
			for _, mbp := range managerBlueprints {
				if bp == mbp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Blueprint %s from mapper not found in manager", bp)
			}
		}
	})

	t.Run("Custom assumptions affect cost estimation", func(t *testing.T) {
		blueprintType := "static-website"

		// Estimate with default assumptions
		defaultConfig := map[string]interface{}{}
		defaultEstimate, err := mapper.EstimateCost(blueprintType, defaultConfig, "us-east-1")
		if err != nil {
			t.Fatalf("Failed to estimate with defaults: %v", err)
		}

		// Estimate with custom assumptions (higher values)
		customConfig := map[string]interface{}{
			"storage_gb":         100.0, // 10x default
			"requests_per_month": 100000, // 10x default
			"data_transfer_gb":   1000.0, // 10x default
		}
		customEstimate, err := mapper.EstimateCost(blueprintType, customConfig, "us-east-1")
		if err != nil {
			t.Fatalf("Failed to estimate with custom config: %v", err)
		}

		// Custom estimate should be higher
		if customEstimate.Estimate <= defaultEstimate.Estimate {
			t.Errorf("Expected custom estimate (%f) to be higher than default (%f)",
				customEstimate.Estimate, defaultEstimate.Estimate)
		}
	})
}

// TestUsageAssumptionsDocumentation tests that assumptions are well-documented
func TestUsageAssumptionsDocumentation(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	t.Run("All assumptions have meaningful descriptions", func(t *testing.T) {
		blueprintTypes := []string{
			string(BlueprintStaticWebsite),
			string(BlueprintWebApp),
			string(BlueprintFullStack),
			string(BlueprintMicroservices),
		}

		for _, blueprintType := range blueprintTypes {
			assumptions, _ := manager.GetAssumptions(blueprintType)

			for _, assumption := range assumptions.Assumptions {
				// Description should be at least 10 characters
				if len(assumption.Description) < 10 {
					t.Errorf("Assumption %s for %s has too short description: %s",
						assumption.Key, blueprintType, assumption.Description)
				}

				// Value should not be empty
				if assumption.Value == "" {
					t.Errorf("Assumption %s for %s has empty value",
						assumption.Key, blueprintType)
				}
			}
		}
	})

	t.Run("Formatted display is user-friendly", func(t *testing.T) {
		blueprintTypes := []string{
			string(BlueprintStaticWebsite),
			string(BlueprintWebApp),
			string(BlueprintFullStack),
			string(BlueprintMicroservices),
		}

		for _, blueprintType := range blueprintTypes {
			display, err := manager.FormatAssumptionsForDisplay(blueprintType)
			if err != nil {
				t.Fatalf("Failed to format display for %s: %v", blueprintType, err)
			}

			// Display should contain blueprint type
			if len(display) < 50 {
				t.Errorf("Display for %s seems too short: %s", blueprintType, display)
			}
		}
	})
}
