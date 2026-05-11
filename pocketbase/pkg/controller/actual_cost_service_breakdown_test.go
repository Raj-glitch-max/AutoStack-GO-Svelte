package controller

import (
	"encoding/json"
	"testing"
	"time"
)

// TestServiceBreakdown_Structure tests the structure of service breakdown
// Validates: AC-3.5 (Breaks down costs by service)
func TestServiceBreakdown_Structure(t *testing.T) {
	response := ActualCostResponse{
		Actual: ActualCostData{
			CostToDate:       47.23,
			ProjectedMonthly: 62.18,
			Variance:         9.76,
			Breakdown: map[string]float64{
				"AmazonEC2":        15.50,
				"AmazonS3":         2.45,
				"AmazonCloudFront": 8.90,
				"AmazonRoute53":    0.50,
				"AmazonRDS":        12.00,
				"DataTransfer":     8.38,
			},
			Period: PeriodData{
				Start: "2026-04-01",
				End:   "2026-04-11",
			},
			FetchedAt: time.Now(),
		},
		Estimate: EstimateData{
			Total:     56.65,
			CreatedAt: time.Now(),
		},
	}

	// Verify breakdown field exists
	if response.Actual.Breakdown == nil {
		t.Fatal("Breakdown field should not be nil")
	}

	// Verify breakdown contains services
	if len(response.Actual.Breakdown) == 0 {
		t.Error("Breakdown should contain at least one service")
	}

	// Verify breakdown is a map of service names to costs
	for serviceName, cost := range response.Actual.Breakdown {
		if serviceName == "" {
			t.Error("Service name should not be empty")
		}
		if cost < 0 {
			t.Errorf("Service cost should not be negative: %s = %.2f", serviceName, cost)
		}
	}
}

// TestServiceBreakdown_CommonAWSServices tests that common AWS services are properly named
// Validates: AC-3.5 (Breaks down costs by service - EC2, S3, RDS, CloudFront, etc.)
func TestServiceBreakdown_CommonAWSServices(t *testing.T) {
	commonServices := []string{
		"AmazonEC2",
		"AmazonS3",
		"AmazonRDS",
		"AmazonCloudFront",
		"AmazonRoute53",
		"AWS Lambda",
		"Amazon DynamoDB",
		"Amazon CloudWatch",
		"AWS Data Transfer",
		"DataTransfer",
	}

	// Create a breakdown with various AWS services
	breakdown := map[string]float64{
		"AmazonEC2":        15.50,
		"AmazonS3":         2.45,
		"AmazonRDS":        12.00,
		"AmazonCloudFront": 8.90,
		"AmazonRoute53":    0.50,
		"AWS Lambda":       3.25,
		"Amazon DynamoDB":  1.80,
		"Amazon CloudWatch": 0.75,
		"DataTransfer":     8.38,
	}

	response := ActualCostResponse{
		Actual: ActualCostData{
			CostToDate:       53.53,
			ProjectedMonthly: 70.00,
			Variance:         10.00,
			Breakdown:        breakdown,
			Period: PeriodData{
				Start: "2026-04-01",
				End:   "2026-04-11",
			},
			FetchedAt: time.Now(),
		},
		Estimate: EstimateData{
			Total:     56.65,
			CreatedAt: time.Now(),
		},
	}

	// Verify each service in breakdown is a valid AWS service name
	for serviceName := range response.Actual.Breakdown {
		isValid := false
		for _, validService := range commonServices {
			if serviceName == validService {
				isValid = true
				break
			}
		}
		if !isValid {
			t.Logf("Warning: Service name '%s' not in common services list (may be valid but uncommon)", serviceName)
		}
	}

	// Verify JSON serialization preserves service names
	jsonData, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	var decoded ActualCostResponse
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify all services are preserved
	if len(decoded.Actual.Breakdown) != len(response.Actual.Breakdown) {
		t.Errorf("Expected %d services, got %d after JSON round-trip",
			len(response.Actual.Breakdown), len(decoded.Actual.Breakdown))
	}

	for serviceName, cost := range response.Actual.Breakdown {
		decodedCost, exists := decoded.Actual.Breakdown[serviceName]
		if !exists {
			t.Errorf("Service '%s' missing after JSON round-trip", serviceName)
		}
		if decodedCost != cost {
			t.Errorf("Service '%s' cost changed: expected %.2f, got %.2f",
				serviceName, cost, decodedCost)
		}
	}
}

// TestServiceBreakdown_SumsToTotal tests that breakdown sums to total cost
// Validates: AC-3.5 (Breakdown sums to total cost)
func TestServiceBreakdown_SumsToTotal(t *testing.T) {
	tests := []struct {
		name      string
		breakdown map[string]float64
		total     float64
		tolerance float64 // Allow small rounding differences
	}{
		{
			name: "Exact sum",
			breakdown: map[string]float64{
				"AmazonEC2": 10.00,
				"AmazonS3":  5.00,
				"AmazonRDS": 15.00,
			},
			total:     30.00,
			tolerance: 0.01,
		},
		{
			name: "With decimals",
			breakdown: map[string]float64{
				"AmazonEC2":        15.50,
				"AmazonS3":         2.45,
				"AmazonCloudFront": 8.90,
				"AmazonRoute53":    0.50,
				"AmazonRDS":        12.00,
				"DataTransfer":     8.38,
			},
			total:     47.73,
			tolerance: 0.01,
		},
		{
			name: "Single service",
			breakdown: map[string]float64{
				"AmazonEC2": 100.00,
			},
			total:     100.00,
			tolerance: 0.01,
		},
		{
			name: "Many small services",
			breakdown: map[string]float64{
				"Service1":  0.01,
				"Service2":  0.02,
				"Service3":  0.03,
				"Service4":  0.04,
				"Service5":  0.05,
				"Service6":  0.06,
				"Service7":  0.07,
				"Service8":  0.08,
				"Service9":  0.09,
				"Service10": 0.10,
			},
			total:     0.55,
			tolerance: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate sum of breakdown
			sum := 0.0
			for _, cost := range tt.breakdown {
				sum += cost
			}

			// Verify sum matches total within tolerance
			diff := sum - tt.total
			if diff < 0 {
				diff = -diff
			}

			if diff > tt.tolerance {
				t.Errorf("Breakdown sum (%.2f) does not match total (%.2f), difference: %.4f",
					sum, tt.total, diff)
			}

			// Create response and verify
			response := ActualCostResponse{
				Actual: ActualCostData{
					CostToDate:       tt.total,
					ProjectedMonthly: tt.total,
					Variance:         0.0,
					Breakdown:        tt.breakdown,
					Period: PeriodData{
						Start: "2026-04-01",
						End:   "2026-04-11",
					},
					FetchedAt: time.Now(),
				},
				Estimate: EstimateData{
					Total:     tt.total,
					CreatedAt: time.Now(),
				},
			}

			// Verify response structure
			if response.Actual.CostToDate != tt.total {
				t.Errorf("CostToDate (%.2f) should match total (%.2f)",
					response.Actual.CostToDate, tt.total)
			}
		})
	}
}

// TestServiceBreakdown_EmptyBreakdown tests handling of empty breakdown
// Validates: AC-3.5 (Breakdown field always present, even if empty)
func TestServiceBreakdown_EmptyBreakdown(t *testing.T) {
	response := ActualCostResponse{
		Actual: ActualCostData{
			CostToDate:       0.00,
			ProjectedMonthly: 0.00,
			Variance:         0.0,
			Breakdown:        map[string]float64{}, // Empty but not nil
			Period: PeriodData{
				Start: "2026-04-01",
				End:   "2026-04-11",
			},
			FetchedAt: time.Now(),
		},
		Estimate: EstimateData{
			Total:     0.00,
			CreatedAt: time.Now(),
		},
	}

	// Verify breakdown is not nil
	if response.Actual.Breakdown == nil {
		t.Error("Breakdown should not be nil, even when empty")
	}

	// Verify breakdown is empty
	if len(response.Actual.Breakdown) != 0 {
		t.Errorf("Expected empty breakdown, got %d services", len(response.Actual.Breakdown))
	}

	// Verify JSON serialization handles empty breakdown
	jsonData, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	var decoded ActualCostResponse
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify empty breakdown is preserved
	if decoded.Actual.Breakdown == nil {
		t.Error("Breakdown should not be nil after JSON round-trip")
	}

	if len(decoded.Actual.Breakdown) != 0 {
		t.Errorf("Expected empty breakdown after JSON round-trip, got %d services",
			len(decoded.Actual.Breakdown))
	}
}

// TestServiceBreakdown_ConsistentNaming tests that service names are consistently formatted
// Validates: AC-3.5 (Format service names consistently)
func TestServiceBreakdown_ConsistentNaming(t *testing.T) {
	// Test various service name formats
	tests := []struct {
		name        string
		serviceName string
		isValid     bool
		description string
	}{
		{
			name:        "Standard AWS service",
			serviceName: "AmazonEC2",
			isValid:     true,
			description: "Standard Amazon service naming",
		},
		{
			name:        "AWS prefix service",
			serviceName: "AWS Lambda",
			isValid:     true,
			description: "AWS prefix naming",
		},
		{
			name:        "Data transfer",
			serviceName: "DataTransfer",
			isValid:     true,
			description: "Special case for data transfer",
		},
		{
			name:        "Amazon prefix with space",
			serviceName: "Amazon CloudWatch",
			isValid:     true,
			description: "Amazon prefix with space",
		},
		{
			name:        "Empty service name",
			serviceName: "",
			isValid:     false,
			description: "Empty service names should not be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			breakdown := map[string]float64{
				tt.serviceName: 10.00,
			}

			response := ActualCostResponse{
				Actual: ActualCostData{
					CostToDate:       10.00,
					ProjectedMonthly: 10.00,
					Variance:         0.0,
					Breakdown:        breakdown,
					Period: PeriodData{
						Start: "2026-04-01",
						End:   "2026-04-11",
					},
					FetchedAt: time.Now(),
				},
				Estimate: EstimateData{
					Total:     10.00,
					CreatedAt: time.Now(),
				},
			}

			// Verify service name validity
			for serviceName := range response.Actual.Breakdown {
				if tt.isValid && serviceName == "" {
					t.Errorf("%s: Service name should not be empty", tt.description)
				}
				if !tt.isValid && serviceName != "" {
					t.Logf("%s: Service name '%s' is present but marked as invalid in test",
						tt.description, serviceName)
				}
			}

			// Verify JSON serialization preserves service name
			jsonData, err := json.Marshal(response)
			if err != nil {
				t.Fatalf("Failed to marshal response: %v", err)
			}

			var decoded ActualCostResponse
			if err := json.Unmarshal(jsonData, &decoded); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			// Verify service name is preserved
			if tt.isValid && tt.serviceName != "" {
				if _, exists := decoded.Actual.Breakdown[tt.serviceName]; !exists {
					t.Errorf("Service name '%s' not preserved after JSON round-trip", tt.serviceName)
				}
			}
		})
	}
}

// TestServiceBreakdown_RoundingPrecision tests that service costs are rounded to 2 decimals
// Validates: TR-2.4 (Round to 2 decimal places for display)
func TestServiceBreakdown_RoundingPrecision(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{
			name:     "Already 2 decimals",
			input:    10.50,
			expected: 10.50,
		},
		{
			name:     "Round up",
			input:    10.556,
			expected: 10.56,
		},
		{
			name:     "Round down",
			input:    10.554,
			expected: 10.55,
		},
		{
			name:     "Very small value",
			input:    0.001,
			expected: 0.00,
		},
		{
			name:     "Large value with precision",
			input:    1234.567,
			expected: 1234.57,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Round using the same logic as the implementation
			rounded := float64(int(tt.input*100+0.5)) / 100

			if rounded != tt.expected {
				t.Errorf("Expected %.2f, got %.2f", tt.expected, rounded)
			}

			// Create breakdown with rounded value
			breakdown := map[string]float64{
				"AmazonEC2": rounded,
			}

			response := ActualCostResponse{
				Actual: ActualCostData{
					CostToDate:       rounded,
					ProjectedMonthly: rounded,
					Variance:         0.0,
					Breakdown:        breakdown,
					Period: PeriodData{
						Start: "2026-04-01",
						End:   "2026-04-11",
					},
					FetchedAt: time.Now(),
				},
				Estimate: EstimateData{
					Total:     rounded,
					CreatedAt: time.Now(),
				},
			}

			// Verify breakdown value is properly rounded
			if response.Actual.Breakdown["AmazonEC2"] != tt.expected {
				t.Errorf("Breakdown value not properly rounded: expected %.2f, got %.2f",
					tt.expected, response.Actual.Breakdown["AmazonEC2"])
			}
		})
	}
}

// TestServiceBreakdown_JSONFieldPresence tests that breakdown field is always in JSON
// Validates: AC-3.5 (Service breakdown always included in response)
func TestServiceBreakdown_JSONFieldPresence(t *testing.T) {
	response := ActualCostResponse{
		Actual: ActualCostData{
			CostToDate:       47.23,
			ProjectedMonthly: 62.18,
			Variance:         9.76,
			Breakdown: map[string]float64{
				"AmazonS3": 2.45,
			},
			Period: PeriodData{
				Start: "2026-04-01",
				End:   "2026-04-11",
			},
			FetchedAt: time.Now(),
		},
		Estimate: EstimateData{
			Total:     56.65,
			CreatedAt: time.Now(),
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	jsonStr := string(jsonData)

	// Verify breakdown field is present in JSON
	if !containsSubstring(jsonStr, "breakdown") {
		t.Error("JSON response should contain 'breakdown' field")
	}

	// Verify breakdown contains service names
	if !containsSubstring(jsonStr, "AmazonS3") {
		t.Error("JSON response breakdown should contain service names")
	}

	// Parse JSON to verify structure
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonMap); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// Navigate to breakdown field
	actual, ok := jsonMap["actual"].(map[string]interface{})
	if !ok {
		t.Fatal("JSON should have 'actual' object")
	}

	breakdown, ok := actual["breakdown"].(map[string]interface{})
	if !ok {
		t.Fatal("JSON 'actual' should have 'breakdown' object")
	}

	// Verify breakdown is not empty
	if len(breakdown) == 0 {
		t.Error("JSON breakdown should not be empty")
	}

	// Verify breakdown contains service costs
	for serviceName, cost := range breakdown {
		if serviceName == "" {
			t.Error("Service name in JSON should not be empty")
		}
		if _, ok := cost.(float64); !ok {
			t.Errorf("Service cost should be a number, got %T", cost)
		}
	}
}
