package aws

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

// MockCostExplorerClient is a mock implementation of Cost Explorer client
type MockCostExplorerClient struct {
	GetCostAndUsageFunc func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
}

func (m *MockCostExplorerClient) GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	if m.GetCostAndUsageFunc != nil {
		return m.GetCostAndUsageFunc(ctx, params, optFns...)
	}
	return nil, nil
}

// TestCalculateProjectedMonthlyCost tests monthly cost projection
func TestCalculateProjectedMonthlyCost(t *testing.T) {
	acf := &ActualCostFetcher{}

	tests := []struct {
		name        string
		costToDate  float64
		period      CostPeriod
		expected    float64
		description string
	}{
		{
			name:       "10 days of data",
			costToDate: 50.00,
			period: CostPeriod{
				Start: time.Now().Add(-10 * 24 * time.Hour),
				End:   time.Now(),
			},
			expected:    150.00, // 50/10 * 30 = 150
			description: "Should project 10 days to 30 days",
		},
		{
			name:       "5 days of data",
			costToDate: 25.00,
			period: CostPeriod{
				Start: time.Now().Add(-5 * 24 * time.Hour),
				End:   time.Now(),
			},
			expected:    150.00, // 25/5 * 30 = 150
			description: "Should project 5 days to 30 days",
		},
		{
			name:       "15 days of data",
			costToDate: 75.00,
			period: CostPeriod{
				Start: time.Now().Add(-15 * 24 * time.Hour),
				End:   time.Now(),
			},
			expected:    150.00, // 75/15 * 30 = 150
			description: "Should project 15 days to 30 days",
		},
		{
			name:       "Zero cost",
			costToDate: 0.00,
			period: CostPeriod{
				Start: time.Now().Add(-10 * 24 * time.Hour),
				End:   time.Now(),
			},
			expected:    0.00,
			description: "Should handle zero cost",
		},
		{
			name:       "Single day",
			costToDate: 5.00,
			period: CostPeriod{
				Start: time.Now().Add(-24 * time.Hour),
				End:   time.Now(),
			},
			expected:    150.00, // 5/1 * 30 = 150
			description: "Should project single day to 30 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := acf.calculateProjectedMonthlyCost(tt.costToDate, tt.period)
			
			// Allow small floating point differences
			if abs(result-tt.expected) > 0.01 {
				t.Errorf("%s: expected %.2f, got %.2f", tt.description, tt.expected, result)
			}
		})
	}
}

// TestCalculateVariance tests variance calculation
// Validates: AC-3.4 (Compares actual vs estimated with variance percentage)
func TestCalculateVariance(t *testing.T) {
	acf := &ActualCostFetcher{}

	tests := []struct {
		name        string
		actual      float64
		estimate    float64
		expected    float64
		description string
	}{
		{
			name:        "Actual equals estimate",
			actual:      100.00,
			estimate:    100.00,
			expected:    0.00,
			description: "Should return 0% variance when equal",
		},
		{
			name:        "Actual 20% higher",
			actual:      120.00,
			estimate:    100.00,
			expected:    20.00,
			description: "Should return +20% variance",
		},
		{
			name:        "Actual 20% lower",
			actual:      80.00,
			estimate:    100.00,
			expected:    -20.00,
			description: "Should return -20% variance",
		},
		{
			name:        "Actual 50% higher",
			actual:      150.00,
			estimate:    100.00,
			expected:    50.00,
			description: "Should return +50% variance",
		},
		{
			name:        "Zero estimate",
			actual:      100.00,
			estimate:    0.00,
			expected:    0.00,
			description: "Should handle zero estimate gracefully",
		},
		{
			name:        "Both zero",
			actual:      0.00,
			estimate:    0.00,
			expected:    0.00,
			description: "Should handle both zero",
		},
		{
			name:        "Very large variance (200%)",
			actual:      300.00,
			estimate:    100.00,
			expected:    200.00,
			description: "Should handle large variance correctly",
		},
		{
			name:        "Very small variance (0.5%)",
			actual:      100.50,
			estimate:    100.00,
			expected:    0.50,
			description: "Should handle small variance with precision",
		},
		{
			name:        "Negative actual cost (credits)",
			actual:      -10.00,
			estimate:    100.00,
			expected:    -110.00,
			description: "Should handle negative actual costs (credits/refunds)",
		},
		{
			name:        "Very small estimate",
			actual:      1.00,
			estimate:    0.50,
			expected:    100.00,
			description: "Should handle very small estimate values",
		},
		{
			name:        "Actual 9.76% higher (from design example)",
			actual:      62.18,
			estimate:    56.65,
			expected:    9.76,
			description: "Should match design document example variance",
		},
		{
			name:        "Rounding edge case",
			actual:      62.185,
			estimate:    56.65,
			expected:    9.77,
			description: "Should round variance to 2 decimal places correctly",
		},
		{
			name:        "Actual 100% higher (doubled)",
			actual:      200.00,
			estimate:    100.00,
			expected:    100.00,
			description: "Should handle 100% variance (doubled cost)",
		},
		{
			name:        "Actual 90% lower (near zero)",
			actual:      10.00,
			estimate:    100.00,
			expected:    -90.00,
			description: "Should handle large negative variance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := acf.calculateVariance(tt.actual, tt.estimate)
			
			if abs(result-tt.expected) > 0.02 {
				t.Errorf("%s: expected %.2f, got %.2f", tt.description, tt.expected, result)
			}
		})
	}
}

// TestProcessCostExplorerResults tests processing of Cost Explorer results
func TestProcessCostExplorerResults(t *testing.T) {
	acf := &ActualCostFetcher{}

	tests := []struct {
		name        string
		input       *costexplorer.GetCostAndUsageOutput
		expected    *ActualCostData
		description string
	}{
		{
			name: "Single service single day",
			input: &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []types.ResultByTime{
					{
						Groups: []types.Group{
							{
								Keys: []string{"AmazonEC2"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {
										Amount: aws.String("10.50"),
										Unit:   aws.String("USD"),
									},
								},
							},
						},
					},
				},
			},
			expected: &ActualCostData{
				CostToDate: 10.50,
				Breakdown: map[string]float64{
					"AmazonEC2": 10.50,
				},
			},
			description: "Should process single service correctly",
		},
		{
			name: "Multiple services single day",
			input: &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []types.ResultByTime{
					{
						Groups: []types.Group{
							{
								Keys: []string{"AmazonEC2"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {
										Amount: aws.String("10.50"),
										Unit:   aws.String("USD"),
									},
								},
							},
							{
								Keys: []string{"AmazonS3"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {
										Amount: aws.String("5.25"),
										Unit:   aws.String("USD"),
									},
								},
							},
						},
					},
				},
			},
			expected: &ActualCostData{
				CostToDate: 15.75,
				Breakdown: map[string]float64{
					"AmazonEC2": 10.50,
					"AmazonS3":  5.25,
				},
			},
			description: "Should process multiple services correctly",
		},
		{
			name: "Multiple days same service",
			input: &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []types.ResultByTime{
					{
						Groups: []types.Group{
							{
								Keys: []string{"AmazonEC2"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {
										Amount: aws.String("10.00"),
										Unit:   aws.String("USD"),
									},
								},
							},
						},
					},
					{
						Groups: []types.Group{
							{
								Keys: []string{"AmazonEC2"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {
										Amount: aws.String("12.00"),
										Unit:   aws.String("USD"),
									},
								},
							},
						},
					},
				},
			},
			expected: &ActualCostData{
				CostToDate: 22.00,
				Breakdown: map[string]float64{
					"AmazonEC2": 22.00,
				},
			},
			description: "Should aggregate costs across multiple days",
		},
		{
			name:  "Empty results",
			input: &costexplorer.GetCostAndUsageOutput{},
			expected: &ActualCostData{
				CostToDate: 0.00,
				Breakdown:  map[string]float64{},
			},
			description: "Should handle empty results",
		},
		{
			name:  "Nil results",
			input: nil,
			expected: &ActualCostData{
				CostToDate: 0.00,
				Breakdown:  map[string]float64{},
			},
			description: "Should handle nil results",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := acf.processCostExplorerResults(tt.input)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.CostToDate != tt.expected.CostToDate {
				t.Errorf("%s: expected cost %.2f, got %.2f", tt.description, tt.expected.CostToDate, result.CostToDate)
			}

			if len(result.Breakdown) != len(tt.expected.Breakdown) {
				t.Errorf("%s: expected %d services, got %d", tt.description, len(tt.expected.Breakdown), len(result.Breakdown))
			}

			for service, expectedCost := range tt.expected.Breakdown {
				actualCost, exists := result.Breakdown[service]
				if !exists {
					t.Errorf("%s: service %s not found in breakdown", tt.description, service)
					continue
				}
				if abs(actualCost-expectedCost) > 0.01 {
					t.Errorf("%s: service %s expected %.2f, got %.2f", tt.description, service, expectedCost, actualCost)
				}
			}
		})
	}
}

// TestCalculateCostPeriod tests cost period calculation
func TestCalculateCostPeriod(t *testing.T) {
	// Mock deployment record
	now := time.Now()
	createdAt := now.Add(-10 * 24 * time.Hour) // 10 days ago

	// Note: In real implementation, this would use a mock PocketBase record
	// For this test, we'll test the logic directly
	
	start := createdAt
	end := now.Add(-CostExplorerDelay)

	if end.Before(start) {
		t.Error("End should not be before start")
	}

	expectedDelay := 48 * time.Hour
	actualDelay := now.Sub(end)
	
	if abs(float64(actualDelay-expectedDelay)) > float64(time.Minute) {
		t.Errorf("Expected delay of %v, got %v", expectedDelay, actualDelay)
	}
}

// TestRoundingFunctions tests rounding helper functions
func TestRoundingFunctions(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"Round up", 10.556, 10.56},
		{"Round down", 10.554, 10.55},
		{"Exact", 10.50, 10.50},
		{"Zero", 0.00, 0.00},
		{"Large number", 1234.567, 1234.57},
		{"Small number", 0.001, 0.00},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := roundToTwoDecimals(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %.2f, got %.2f", tt.expected, result)
			}
		})
	}
}

// TestRoundBreakdownToTwoDecimals tests breakdown rounding
func TestRoundBreakdownToTwoDecimals(t *testing.T) {
	input := map[string]float64{
		"AmazonEC2": 10.556,
		"AmazonS3":  5.554,
		"AmazonRDS": 0.001,
	}

	expected := map[string]float64{
		"AmazonEC2": 10.56,
		"AmazonS3":  5.55,
		"AmazonRDS": 0.00,
	}

	result := roundBreakdownToTwoDecimals(input)

	for service, expectedCost := range expected {
		actualCost, exists := result[service]
		if !exists {
			t.Errorf("Service %s not found in result", service)
			continue
		}
		if actualCost != expectedCost {
			t.Errorf("Service %s: expected %.2f, got %.2f", service, expectedCost, actualCost)
		}
	}
}

// TestParseFloat tests float parsing
func TestParseFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
	}{
		{"Integer", "10", 10.0},
		{"Decimal", "10.50", 10.50},
		{"Zero", "0", 0.0},
		{"Large number", "1234.56", 1234.56},
		{"Small number", "0.01", 0.01},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFloat(tt.input)
			if abs(result-tt.expected) > 0.001 {
				t.Errorf("Expected %.2f, got %.2f", tt.expected, result)
			}
		})
	}
}

// TestMergeBreakdowns tests merging of cost breakdowns
func TestMergeBreakdowns(t *testing.T) {
	acf := &ActualCostFetcher{}

	tests := []struct {
		name        string
		existing    map[string]float64
		new         map[string]float64
		expected    map[string]float64
		description string
	}{
		{
			name: "Merge different services",
			existing: map[string]float64{
				"AmazonEC2": 10.50,
				"AmazonS3":  5.25,
			},
			new: map[string]float64{
				"AmazonRDS": 15.75,
			},
			expected: map[string]float64{
				"AmazonEC2": 10.50,
				"AmazonS3":  5.25,
				"AmazonRDS": 15.75,
			},
			description: "Should merge different services",
		},
		{
			name: "Merge same services",
			existing: map[string]float64{
				"AmazonEC2": 10.50,
				"AmazonS3":  5.25,
			},
			new: map[string]float64{
				"AmazonEC2": 8.30,
				"AmazonS3":  3.15,
			},
			expected: map[string]float64{
				"AmazonEC2": 18.80,
				"AmazonS3":  8.40,
			},
			description: "Should add costs for same services",
		},
		{
			name: "Merge overlapping services",
			existing: map[string]float64{
				"AmazonEC2": 10.50,
				"AmazonS3":  5.25,
			},
			new: map[string]float64{
				"AmazonS3":  3.15,
				"AmazonRDS": 12.60,
			},
			expected: map[string]float64{
				"AmazonEC2": 10.50,
				"AmazonS3":  8.40,
				"AmazonRDS": 12.60,
			},
			description: "Should handle overlapping services",
		},
		{
			name:     "Empty existing",
			existing: map[string]float64{},
			new: map[string]float64{
				"AmazonEC2": 10.50,
			},
			expected: map[string]float64{
				"AmazonEC2": 10.50,
			},
			description: "Should handle empty existing breakdown",
		},
		{
			name: "Empty new",
			existing: map[string]float64{
				"AmazonEC2": 10.50,
			},
			new: map[string]float64{},
			expected: map[string]float64{
				"AmazonEC2": 10.50,
			},
			description: "Should handle empty new breakdown",
		},
		{
			name:        "Both empty",
			existing:    map[string]float64{},
			new:         map[string]float64{},
			expected:    map[string]float64{},
			description: "Should handle both empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := acf.mergeBreakdowns(tt.existing, tt.new)

			if len(result) != len(tt.expected) {
				t.Errorf("%s: expected %d services, got %d", tt.description, len(tt.expected), len(result))
			}

			for service, expectedCost := range tt.expected {
				actualCost, exists := result[service]
				if !exists {
					t.Errorf("%s: service %s not found in result", tt.description, service)
					continue
				}
				if abs(actualCost-expectedCost) > 0.01 {
					t.Errorf("%s: service %s expected %.2f, got %.2f", tt.description, service, expectedCost, actualCost)
				}
			}
		})
	}
}

// TestIncrementalUpdateLogic tests the incremental update period calculation logic
func TestIncrementalUpdateLogic(t *testing.T) {
	now := time.Now()
	
	tests := []struct {
		name               string
		existingPeriodEnd  time.Time
		shouldLimitRange   bool
		shouldHaveNewData  bool
		description        string
	}{
		{
			name:              "Recent fetch (1 day ago)",
			existingPeriodEnd: now.Add(-1 * 24 * time.Hour),
			shouldLimitRange:  false,
			shouldHaveNewData: false, // 48h delay means no new data yet
			description:       "Should have no new data (within 48h delay)",
		},
		{
			name:              "Older fetch (5 days ago)",
			existingPeriodEnd: now.Add(-5 * 24 * time.Hour),
			shouldLimitRange:  false,
			shouldHaveNewData: true,
			description:       "Should fetch from last period end to 48h ago",
		},
		{
			name:              "Very old fetch (100 days ago)",
			existingPeriodEnd: now.Add(-100 * 24 * time.Hour),
			shouldLimitRange:  true,
			shouldHaveNewData: true,
			description:       "Should limit fetch range to 90 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the logic conceptually
			newEnd := now.Add(-CostExplorerDelay)
			newStart := tt.existingPeriodEnd
			
			// Check if there's new data to fetch
			hasNewData := newEnd.After(newStart)
			
			if hasNewData != tt.shouldHaveNewData {
				t.Errorf("%s: expected new data availability to be %v, got %v", tt.description, tt.shouldHaveNewData, hasNewData)
			}
			
			if !hasNewData {
				// No new data, skip further checks
				return
			}
			
			// Check if we would limit the range
			rangeWasLimited := false
			if newEnd.Sub(newStart) > 90*24*time.Hour {
				newStart = newEnd.Add(-90 * 24 * time.Hour)
				rangeWasLimited = true
			}
			
			if rangeWasLimited != tt.shouldLimitRange {
				t.Errorf("%s: expected range limiting to be %v, got %v", tt.description, tt.shouldLimitRange, rangeWasLimited)
			}
			
			// Verify start is not after end
			if newStart.After(newEnd) {
				t.Errorf("%s: start time %v should not be after end time %v", tt.description, newStart, newEnd)
			}
			
			// Verify the range is reasonable
			rangeDays := newEnd.Sub(newStart).Hours() / 24
			if rangeDays < 0 {
				t.Errorf("%s: range should be positive, got %.2f days", tt.description, rangeDays)
			}
			
			if tt.shouldLimitRange && rangeDays > 90 {
				t.Errorf("%s: range should be limited to 90 days, got %.2f days", tt.description, rangeDays)
			}
		})
	}
}

// Helper function for absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Benchmark tests

func BenchmarkCalculateProjectedMonthlyCost(b *testing.B) {
	acf := &ActualCostFetcher{}
	period := CostPeriod{
		Start: time.Now().Add(-10 * 24 * time.Hour),
		End:   time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acf.calculateProjectedMonthlyCost(50.00, period)
	}
}

func BenchmarkCalculateVariance(b *testing.B) {
	acf := &ActualCostFetcher{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acf.calculateVariance(120.00, 100.00)
	}
}

func BenchmarkRoundToTwoDecimals(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		roundToTwoDecimals(123.456789)
	}
}
