package aws

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/pocketbase/pocketbase/tests"
)

// MockCostExplorerClientForIntegration is a mock implementation for integration tests
type MockCostExplorerClientForIntegration struct {
	GetCostAndUsageFunc func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
}

func (m *MockCostExplorerClientForIntegration) GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	if m.GetCostAndUsageFunc != nil {
		return m.GetCostAndUsageFunc(ctx, params, optFns...)
	}
	return &costexplorer.GetCostAndUsageOutput{}, nil
}

// TestActualCostFetcherIntegration tests the full flow of fetching actual costs
func TestActualCostFetcherIntegration(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Create mock Cost Explorer client
	mockClient := &MockCostExplorerClientForIntegration{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			// Return mock cost data
			return &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []types.ResultByTime{
					{
						TimePeriod: &types.DateInterval{
							Start: aws.String("2024-01-01"),
							End:   aws.String("2024-01-02"),
						},
						Groups: []types.Group{
							{
								Keys: []string{"AmazonEC2"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {
										Amount: aws.String("25.50"),
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
					{
						TimePeriod: &types.DateInterval{
							Start: aws.String("2024-01-02"),
							End:   aws.String("2024-01-03"),
						},
						Groups: []types.Group{
							{
								Keys: []string{"AmazonEC2"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {
										Amount: aws.String("26.00"),
										Unit:   aws.String("USD"),
									},
								},
							},
							{
								Keys: []string{"AmazonS3"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {
										Amount: aws.String("5.50"),
										Unit:   aws.String("USD"),
									},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	// Create ActualCostFetcher with mock client
	acf := &ActualCostFetcher{
		app:    app,
		client: mockClient,
		retry:  NewRetryManager(DefaultRetryConfig()),
	}

	// Test processing Cost Explorer results
	t.Run("Process Cost Explorer Results", func(t *testing.T) {
		result, err := mockClient.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{})
		if err != nil {
			t.Fatalf("Failed to get cost data: %v", err)
		}

		costData, err := acf.processCostExplorerResults(result)
		if err != nil {
			t.Fatalf("Failed to process results: %v", err)
		}

		// Verify total cost (25.50 + 5.25 + 26.00 + 5.50 = 62.25)
		expectedTotal := 62.25
		if costData.CostToDate != expectedTotal {
			t.Errorf("Expected total cost %.2f, got %.2f", expectedTotal, costData.CostToDate)
		}

		// Verify breakdown
		expectedBreakdown := map[string]float64{
			"AmazonEC2": 51.50, // 25.50 + 26.00
			"AmazonS3":  10.75, // 5.25 + 5.50
		}

		for service, expectedCost := range expectedBreakdown {
			actualCost, exists := costData.Breakdown[service]
			if !exists {
				t.Errorf("Service %s not found in breakdown", service)
				continue
			}
			if abs(actualCost-expectedCost) > 0.01 {
				t.Errorf("Service %s: expected %.2f, got %.2f", service, expectedCost, actualCost)
			}
		}
	})

	// Test monthly projection
	t.Run("Calculate Monthly Projection", func(t *testing.T) {
		costToDate := 62.25
		period := CostPeriod{
			Start: time.Now().Add(-10 * 24 * time.Hour),
			End:   time.Now(),
		}

		projected := acf.calculateProjectedMonthlyCost(costToDate, period)
		
		// 62.25 / 10 days * 30 days = 186.75
		expected := 186.75
		if abs(projected-expected) > 0.01 {
			t.Errorf("Expected projected monthly cost %.2f, got %.2f", expected, projected)
		}
	})

	// Test variance calculation
	t.Run("Calculate Variance", func(t *testing.T) {
		actual := 186.75
		estimate := 150.00
		
		variance := acf.calculateVariance(actual, estimate)
		
		// (186.75 - 150) / 150 * 100 = 24.5%
		expected := 24.50
		if abs(variance-expected) > 0.02 {
			t.Errorf("Expected variance %.2f%%, got %.2f%%", expected, variance)
		}
	})
}

// TestActualCostFetcherErrorHandling tests error handling scenarios
func TestActualCostFetcherErrorHandling(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	t.Run("Handle Empty Cost Explorer Results", func(t *testing.T) {
		mockClient := &MockCostExplorerClientForIntegration{
			GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
				return &costexplorer.GetCostAndUsageOutput{
					ResultsByTime: []types.ResultByTime{},
				}, nil
			},
		}

		acf := &ActualCostFetcher{
			app:    app,
			client: mockClient,
			retry:  NewRetryManager(DefaultRetryConfig()),
		}

		result, err := mockClient.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{})
		if err != nil {
			t.Fatalf("Failed to get cost data: %v", err)
		}

		costData, err := acf.processCostExplorerResults(result)
		if err != nil {
			t.Fatalf("Failed to process empty results: %v", err)
		}

		if costData.CostToDate != 0 {
			t.Errorf("Expected zero cost for empty results, got %.2f", costData.CostToDate)
		}

		if len(costData.Breakdown) != 0 {
			t.Errorf("Expected empty breakdown, got %d services", len(costData.Breakdown))
		}
	})

	t.Run("Handle Nil Cost Explorer Results", func(t *testing.T) {
		acf := &ActualCostFetcher{
			app:   app,
			retry: NewRetryManager(DefaultRetryConfig()),
		}

		costData, err := acf.processCostExplorerResults(nil)
		if err != nil {
			t.Fatalf("Failed to process nil results: %v", err)
		}

		if costData.CostToDate != 0 {
			t.Errorf("Expected zero cost for nil results, got %.2f", costData.CostToDate)
		}
	})

	t.Run("Handle Zero Estimate in Variance Calculation", func(t *testing.T) {
		acf := &ActualCostFetcher{
			app:   app,
			retry: NewRetryManager(DefaultRetryConfig()),
		}

		variance := acf.calculateVariance(100.00, 0.00)
		
		if variance != 0 {
			t.Errorf("Expected zero variance for zero estimate, got %.2f", variance)
		}
	})
}

// TestActualCostFetcherEdgeCases tests edge cases
func TestActualCostFetcherEdgeCases(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	acf := &ActualCostFetcher{
		app:   app,
		retry: NewRetryManager(DefaultRetryConfig()),
	}

	t.Run("Very Small Costs", func(t *testing.T) {
		mockClient := &MockCostExplorerClientForIntegration{
			GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
				return &costexplorer.GetCostAndUsageOutput{
					ResultsByTime: []types.ResultByTime{
						{
							Groups: []types.Group{
								{
									Keys: []string{"AmazonS3"},
									Metrics: map[string]types.MetricValue{
										"BlendedCost": {
											Amount: aws.String("0.001"),
											Unit:   aws.String("USD"),
										},
									},
								},
							},
						},
					},
				}, nil
			},
		}

		acf.client = mockClient
		result, _ := mockClient.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{})
		costData, err := acf.processCostExplorerResults(result)
		
		if err != nil {
			t.Fatalf("Failed to process small costs: %v", err)
		}

		// Should round to 0.00
		if costData.CostToDate != 0.00 {
			t.Errorf("Expected 0.00 for very small cost, got %.2f", costData.CostToDate)
		}
	})

	t.Run("Very Large Costs", func(t *testing.T) {
		mockClient := &MockCostExplorerClientForIntegration{
			GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
				return &costexplorer.GetCostAndUsageOutput{
					ResultsByTime: []types.ResultByTime{
						{
							Groups: []types.Group{
								{
									Keys: []string{"AmazonEC2"},
									Metrics: map[string]types.MetricValue{
										"BlendedCost": {
											Amount: aws.String("12345.678"),
											Unit:   aws.String("USD"),
										},
									},
								},
							},
						},
					},
				}, nil
			},
		}

		acf.client = mockClient
		result, _ := mockClient.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{})
		costData, err := acf.processCostExplorerResults(result)
		
		if err != nil {
			t.Fatalf("Failed to process large costs: %v", err)
		}

		// Should round to 12345.68
		expected := 12345.68
		if costData.CostToDate != expected {
			t.Errorf("Expected %.2f for large cost, got %.2f", expected, costData.CostToDate)
		}
	})

	t.Run("Single Day Period", func(t *testing.T) {
		costToDate := 10.00
		period := CostPeriod{
			Start: time.Now().Add(-24 * time.Hour),
			End:   time.Now(),
		}

		projected := acf.calculateProjectedMonthlyCost(costToDate, period)
		
		// 10 / 1 * 30 = 300
		expected := 300.00
		if abs(projected-expected) > 0.01 {
			t.Errorf("Expected %.2f for single day projection, got %.2f", expected, projected)
		}
	})

	t.Run("Full Month Period", func(t *testing.T) {
		costToDate := 300.00
		period := CostPeriod{
			Start: time.Now().Add(-30 * 24 * time.Hour),
			End:   time.Now(),
		}

		projected := acf.calculateProjectedMonthlyCost(costToDate, period)
		
		// 300 / 30 * 30 = 300
		expected := 300.00
		if abs(projected-expected) > 0.01 {
			t.Errorf("Expected %.2f for full month projection, got %.2f", expected, projected)
		}
	})
}

// TestActualCostFetcherServiceBreakdown tests service-level cost breakdown
func TestActualCostFetcherServiceBreakdown(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	mockClient := &MockCostExplorerClientForIntegration{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []types.ResultByTime{
					{
						Groups: []types.Group{
							{
								Keys: []string{"AmazonEC2"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {Amount: aws.String("50.00"), Unit: aws.String("USD")},
								},
							},
							{
								Keys: []string{"AmazonS3"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {Amount: aws.String("10.00"), Unit: aws.String("USD")},
								},
							},
							{
								Keys: []string{"AmazonRDS"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {Amount: aws.String("30.00"), Unit: aws.String("USD")},
								},
							},
							{
								Keys: []string{"AmazonCloudFront"},
								Metrics: map[string]types.MetricValue{
									"BlendedCost": {Amount: aws.String("5.00"), Unit: aws.String("USD")},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	acf := &ActualCostFetcher{
		app:    app,
		client: mockClient,
		retry:  NewRetryManager(DefaultRetryConfig()),
	}

	result, _ := mockClient.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{})
	costData, err := acf.processCostExplorerResults(result)
	
	if err != nil {
		t.Fatalf("Failed to process service breakdown: %v", err)
	}

	// Verify total
	expectedTotal := 95.00
	if costData.CostToDate != expectedTotal {
		t.Errorf("Expected total %.2f, got %.2f", expectedTotal, costData.CostToDate)
	}

	// Verify each service
	expectedBreakdown := map[string]float64{
		"AmazonEC2":        50.00,
		"AmazonS3":         10.00,
		"AmazonRDS":        30.00,
		"AmazonCloudFront": 5.00,
	}

	if len(costData.Breakdown) != len(expectedBreakdown) {
		t.Errorf("Expected %d services, got %d", len(expectedBreakdown), len(costData.Breakdown))
	}

	for service, expectedCost := range expectedBreakdown {
		actualCost, exists := costData.Breakdown[service]
		if !exists {
			t.Errorf("Service %s not found in breakdown", service)
			continue
		}
		if actualCost != expectedCost {
			t.Errorf("Service %s: expected %.2f, got %.2f", service, expectedCost, actualCost)
		}
	}
}
