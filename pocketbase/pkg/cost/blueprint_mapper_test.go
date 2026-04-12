package cost

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestBlueprintMapper(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	mapper := NewBlueprintMapper(app)

	t.Run("GetSupportedBlueprints returns all blueprint types", func(t *testing.T) {
		blueprints := mapper.GetSupportedBlueprints()
		
		if len(blueprints) != 4 {
			t.Errorf("Expected 4 supported blueprints, got %d", len(blueprints))
		}

		expectedBlueprints := []string{
			"static-website",
			"web-application",
			"full-stack-app",
			"microservices",
		}

		for _, expected := range expectedBlueprints {
			found := false
			for _, bp := range blueprints {
				if bp == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected blueprint %s not found in supported list", expected)
			}
		}
	})

	t.Run("IsBlueprintSupported validates blueprint types", func(t *testing.T) {
		testCases := []struct {
			blueprintType string
			expected      bool
		}{
			{"static-website", true},
			{"web-application", true},
			{"full-stack-app", true},
			{"microservices", true},
			{"invalid-blueprint", false},
			{"", false},
		}

		for _, tc := range testCases {
			result := mapper.IsBlueprintSupported(tc.blueprintType)
			if result != tc.expected {
				t.Errorf("IsBlueprintSupported(%s) = %v, expected %v", tc.blueprintType, result, tc.expected)
			}
		}
	})

	t.Run("GetAssumptions returns assumptions for each blueprint", func(t *testing.T) {
		blueprints := []string{
			"static-website",
			"web-application",
			"full-stack-app",
			"microservices",
		}

		for _, bp := range blueprints {
			assumptions, err := mapper.GetAssumptions(bp)
			if err != nil {
				t.Errorf("GetAssumptions(%s) returned error: %v", bp, err)
			}
			if len(assumptions) == 0 {
				t.Errorf("GetAssumptions(%s) returned empty assumptions", bp)
			}
		}
	})

	t.Run("GetDisclaimer returns disclaimer for each blueprint", func(t *testing.T) {
		blueprints := []string{
			"static-website",
			"web-application",
			"full-stack-app",
			"microservices",
		}

		for _, bp := range blueprints {
			disclaimer, err := mapper.GetDisclaimer(bp)
			if err != nil {
				t.Errorf("GetDisclaimer(%s) returned error: %v", bp, err)
			}
			if disclaimer == "" {
				t.Errorf("GetDisclaimer(%s) returned empty disclaimer", bp)
			}
		}
	})

	t.Run("EstimateCost calculates cost for static website", func(t *testing.T) {
		config := map[string]interface{}{
			"storage_gb":         10.0,
			"requests_per_month": 10000,
			"data_transfer_gb":   100.0,
		}

		estimate, err := mapper.EstimateCost("static-website", config, "us-east-1")
		if err != nil {
			t.Fatalf("EstimateCost failed: %v", err)
		}

		if estimate.Estimate <= 0 {
			t.Errorf("Expected positive estimate, got %f", estimate.Estimate)
		}
		if estimate.RangeMin > estimate.Estimate {
			t.Errorf("RangeMin (%f) should be <= Estimate (%f)", estimate.RangeMin, estimate.Estimate)
		}
		if estimate.RangeMax < estimate.Estimate {
			t.Errorf("RangeMax (%f) should be >= Estimate (%f)", estimate.RangeMax, estimate.Estimate)
		}
	})

	t.Run("EstimateCost calculates cost for web application", func(t *testing.T) {
		config := map[string]interface{}{
			"vcpu":       0.25,
			"memory":     0.5,
			"task_count": 1,
		}

		estimate, err := mapper.EstimateCost("web-application", config, "us-east-1")
		if err != nil {
			t.Fatalf("EstimateCost failed: %v", err)
		}

		if estimate.Estimate <= 0 {
			t.Errorf("Expected positive estimate, got %f", estimate.Estimate)
		}
	})

	t.Run("EstimateCost merges with defaults", func(t *testing.T) {
		// Provide minimal config, should use defaults for missing values
		config := map[string]interface{}{}

		estimate, err := mapper.EstimateCost("static-website", config, "us-east-1")
		if err != nil {
			t.Fatalf("EstimateCost with empty config failed: %v", err)
		}

		if estimate.Estimate <= 0 {
			t.Errorf("Expected positive estimate with defaults, got %f", estimate.Estimate)
		}
	})

	t.Run("EstimateCost fails for unsupported blueprint", func(t *testing.T) {
		config := map[string]interface{}{}

		_, err := mapper.EstimateCost("invalid-blueprint", config, "us-east-1")
		if err == nil {
			t.Error("Expected error for unsupported blueprint, got nil")
		}
	})

	t.Run("GetBlueprintDescription returns description", func(t *testing.T) {
		blueprints := []string{
			"static-website",
			"web-application",
			"full-stack-app",
			"microservices",
		}

		for _, bp := range blueprints {
			desc, err := mapper.GetBlueprintDescription(bp)
			if err != nil {
				t.Errorf("GetBlueprintDescription(%s) returned error: %v", bp, err)
			}
			if desc == "" {
				t.Errorf("GetBlueprintDescription(%s) returned empty description", bp)
			}
		}
	})

	t.Run("GetServiceBreakdown returns services list", func(t *testing.T) {
		blueprints := []string{
			"static-website",
			"web-application",
			"full-stack-app",
			"microservices",
		}

		for _, bp := range blueprints {
			services, err := mapper.GetServiceBreakdown(bp)
			if err != nil {
				t.Errorf("GetServiceBreakdown(%s) returned error: %v", bp, err)
			}
			if len(services) == 0 {
				t.Errorf("GetServiceBreakdown(%s) returned empty services list", bp)
			}
		}
	})

	t.Run("ValidateConfig validates blueprint configuration", func(t *testing.T) {
		testCases := []struct {
			name          string
			blueprintType string
			config        map[string]interface{}
			expectError   bool
		}{
			{
				name:          "valid static website config",
				blueprintType: "static-website",
				config: map[string]interface{}{
					"region":             "us-east-1",
					"storage_gb":         10.0,
					"requests_per_month": 10000,
				},
				expectError: false,
			},
			{
				name:          "invalid static website config - negative storage",
				blueprintType: "static-website",
				config: map[string]interface{}{
					"region":     "us-east-1",
					"storage_gb": -10.0,
				},
				expectError: true,
			},
			{
				name:          "valid web app config",
				blueprintType: "web-application",
				config: map[string]interface{}{
					"region":     "us-east-1",
					"vcpu":       0.25,
					"task_count": 1,
				},
				expectError: false,
			},
			{
				name:          "invalid web app config - invalid vCPU",
				blueprintType: "web-application",
				config: map[string]interface{}{
					"region": "us-east-1",
					"vcpu":   0.3,
				},
				expectError: true,
			},
			{
				name:          "missing region",
				blueprintType: "static-website",
				config:        map[string]interface{}{},
				expectError:   true,
			},
			{
				name:          "unsupported blueprint",
				blueprintType: "invalid-blueprint",
				config: map[string]interface{}{
					"region": "us-east-1",
				},
				expectError: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := mapper.ValidateConfig(tc.blueprintType, tc.config)
				if tc.expectError && err == nil {
					t.Error("Expected error but got nil")
				}
				if !tc.expectError && err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			})
		}
	})
}
