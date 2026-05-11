package aws

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/pocketbase/pocketbase/tests"
)

// MockPricingClient implements a mock AWS Pricing client for testing
type MockPricingClient struct {
	GetProductsFunc func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
}

func (m *MockPricingClient) GetProducts(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
	if m.GetProductsFunc != nil {
		return m.GetProductsFunc(ctx, params, optFns...)
	}
	return &pricing.GetProductsOutput{}, nil
}

// createMockPricingFetcher creates a pricing fetcher with a mock client and an
// in-memory PocketBase test app that has the awsPricingCache collection seeded.
//
// Tests in this package are integration tests that exercise real DB round-trips
// against an in-memory SQLite instance. The collection schema must be bootstrapped
// here because tests.NewTestApp() starts with a blank schema (no migrations).
func createMockPricingFetcher(t *testing.T, mockClient *MockPricingClient) *PricingFetcher {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	// Bootstrap the awsPricingCache collection schema into the blank test DB.
	// This mirrors what the production JS migration does.
	if err := bootstrapPricingCacheCollection(app); err != nil {
		t.Fatalf("failed to bootstrap awsPricingCache collection: %v", err)
	}

	pf := &PricingFetcher{
		app:       app,
		client:    mockClient,
		regions:   []string{"us-east-1", "us-west-2"},
		batchSize: 100,
	}

	return pf
}


func TestPricingFetcher_FetchFargateRates(t *testing.T) {
	mockClient := &MockPricingClient{
		GetProductsFunc: func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
			// Return mock Fargate pricing data
			mockProduct := `{
				"product": {
					"attributes": {
						"vcpu": "0.25",
						"memory": "0.5 GB"
					}
				},
				"terms": {
					"OnDemand": {
						"term1": {
							"priceDimensions": {
								"dimension1": {
									"pricePerUnit": {
										"USD": "0.04048"
									}
								}
							}
						}
					}
				}
			}`

			return &pricing.GetProductsOutput{
				PriceList: []string{mockProduct},
			}, nil
		},
	}

	pf := createMockPricingFetcher(t, mockClient)

	// Test fetching Fargate rates
	err := pf.fetchFargateRates()
	if err != nil {
		t.Errorf("fetchFargateRates() failed: %v", err)
	}

	// Verify data was saved to database.
	// Use t.Fatalf (not t.Errorf) so the test stops here if GetFargateRates fails —
	// continuing past a nil `rates` would panic.
	rates, err := pf.GetFargateRates("us-east-1")
	if err != nil {
		t.Fatalf("GetFargateRates() failed: %v", err)
	}

	if rates.VCPUPricePerHour != 0.04048 {
		t.Errorf("Expected vCPU price 0.04048, got %f", rates.VCPUPricePerHour)
	}
}

func TestPricingFetcher_FetchS3Rates(t *testing.T) {
	mockClient := &MockPricingClient{
		GetProductsFunc: func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
			// Return mock S3 pricing data
			mockProduct := `{
				"terms": {
					"OnDemand": {
						"term1": {
							"priceDimensions": {
								"dimension1": {
									"pricePerUnit": {
										"USD": "0.023"
									}
								}
							}
						}
					}
				}
			}`

			return &pricing.GetProductsOutput{
				PriceList: []string{mockProduct},
			}, nil
		},
	}

	pf := createMockPricingFetcher(t, mockClient)

	// Test fetching S3 rates
	err := pf.fetchS3Rates()
	if err != nil {
		t.Errorf("fetchS3Rates() failed: %v", err)
	}

	// Verify data was saved to database.
	storageRate, err := pf.GetS3StorageRates("us-east-1")
	if err != nil {
		t.Fatalf("GetS3StorageRates() failed: %v", err)
	}

	if storageRate <= 0 {
		t.Errorf("Expected positive storage rate, got %f", storageRate)
	}
}

func TestPricingFetcher_FetchALBRates(t *testing.T) {
	mockClient := &MockPricingClient{
		GetProductsFunc: func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
			// Return mock ALB pricing data
			mockProduct := `{
				"terms": {
					"OnDemand": {
						"term1": {
							"priceDimensions": {
								"dimension1": {
									"pricePerUnit": {
										"USD": "0.0225"
									}
								}
							}
						}
					}
				}
			}`

			return &pricing.GetProductsOutput{
				PriceList: []string{mockProduct},
			}, nil
		},
	}

	pf := createMockPricingFetcher(t, mockClient)

	// Test fetching ALB rates
	err := pf.fetchALBRates()
	if err != nil {
		t.Errorf("fetchALBRates() failed: %v", err)
	}

	// Verify data was saved to database.
	rates, err := pf.GetALBRates("us-east-1")
	if err != nil {
		t.Fatalf("GetALBRates() failed: %v", err)
	}

	if rates.HourlyRate != 0.0225 {
		t.Errorf("Expected ALB hourly rate 0.0225, got %f", rates.HourlyRate)
	}
}

func TestPricingFetcher_FetchRDSRates(t *testing.T) {
	mockClient := &MockPricingClient{
		GetProductsFunc: func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
			// Return mock RDS pricing data
			mockProduct := `{
				"terms": {
					"OnDemand": {
						"term1": {
							"priceDimensions": {
								"dimension1": {
									"pricePerUnit": {
										"USD": "0.017"
									}
								}
							}
						}
					}
				}
			}`

			return &pricing.GetProductsOutput{
				PriceList: []string{mockProduct},
			}, nil
		},
	}

	pf := createMockPricingFetcher(t, mockClient)

	// Test fetching RDS rates
	err := pf.fetchRDSRates()
	if err != nil {
		t.Errorf("fetchRDSRates() failed: %v", err)
	}

	// Verify data was saved to database.
	instanceRate, err := pf.GetRDSInstanceRates("us-east-1", "db.t3.micro", "MySQL")
	if err != nil {
		t.Fatalf("GetRDSInstanceRates() failed: %v", err)
	}

	if instanceRate != 0.017 {
		t.Errorf("Expected RDS instance rate 0.017, got %f", instanceRate)
	}
}

func TestPricingFetcher_CostCalculations(t *testing.T) {
	mockClient := &MockPricingClient{
		GetProductsFunc: func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
			// Return different mock data based on service
			serviceCode := *params.ServiceCode
			
			var mockProduct string
			switch serviceCode {
			case "AmazonECS":
				mockProduct = `{
					"terms": {
						"OnDemand": {
							"term1": {
								"priceDimensions": {
									"dimension1": {
										"pricePerUnit": {
											"USD": "0.04048"
										}
									}
								}
							}
						}
					}
				}`
			case "AmazonS3":
				mockProduct = `{
					"terms": {
						"OnDemand": {
							"term1": {
								"priceDimensions": {
									"dimension1": {
										"pricePerUnit": {
											"USD": "0.023"
										}
									}
								}
							}
						}
					}
				}`
			default:
				mockProduct = `{
					"terms": {
						"OnDemand": {
							"term1": {
								"priceDimensions": {
									"dimension1": {
										"pricePerUnit": {
											"USD": "0.01"
										}
									}
								}
							}
						}
					}
				}`
			}

			return &pricing.GetProductsOutput{
				PriceList: []string{mockProduct},
			}, nil
		},
	}

	pf := createMockPricingFetcher(t, mockClient)

	// Fetch all pricing data
	if err := pf.fetchFargateRates(); err != nil {
		t.Errorf("fetchFargateRates() failed: %v", err)
	}
	if err := pf.fetchS3Rates(); err != nil {
		t.Errorf("fetchS3Rates() failed: %v", err)
	}

	// Test Fargate cost calculation
	fargateCost, err := pf.CalculateFargateCost("us-east-1", 0.25, 0.5)
	if err != nil {
		t.Errorf("CalculateFargateCost() failed: %v", err)
	}
	if fargateCost <= 0 {
		t.Errorf("Expected positive Fargate cost, got %f", fargateCost)
	}

	// Test S3 cost calculation
	s3Cost, err := pf.CalculateS3Cost("us-east-1", 10.0, 1000, 5000)
	if err != nil {
		t.Errorf("CalculateS3Cost() failed: %v", err)
	}
	if s3Cost <= 0 {
		t.Errorf("Expected positive S3 cost, got %f", s3Cost)
	}
}

func TestPricingFetcher_DataFreshness(t *testing.T) {
	mockClient := &MockPricingClient{
		GetProductsFunc: func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
			mockProduct := `{
				"terms": {
					"OnDemand": {
						"term1": {
							"priceDimensions": {
								"dimension1": {
									"pricePerUnit": {
										"USD": "0.04048"
									}
								}
							}
						}
					}
				}
			}`

			return &pricing.GetProductsOutput{
				PriceList: []string{mockProduct},
			}, nil
		},
	}

	pf := createMockPricingFetcher(t, mockClient)

	// Test data staleness check with no data
	isStale, err := pf.IsDataStale(24 * time.Hour)
	if err == nil {
		t.Error("Expected error when no pricing data exists")
	}

	// Fetch some data
	if err := pf.fetchFargateRates(); err != nil {
		t.Errorf("fetchFargateRates() failed: %v", err)
	}

	// Test data staleness check with fresh data
	isStale, err = pf.IsDataStale(24 * time.Hour)
	if err != nil {
		t.Errorf("IsDataStale() failed: %v", err)
	}
	if isStale {
		t.Error("Expected fresh data to not be stale")
	}

	// Test data staleness check with very short max age
	isStale, err = pf.IsDataStale(1 * time.Nanosecond)
	if err != nil {
		t.Errorf("IsDataStale() failed: %v", err)
	}
	if !isStale {
		t.Error("Expected data to be stale with very short max age")
	}
}

func TestPricingFetcher_ErrorHandling(t *testing.T) {
	// Test with client that returns errors
	mockClient := &MockPricingClient{
		GetProductsFunc: func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
			return nil, &types.ThrottlingException{
				Message: aws.String("Rate exceeded"),
			}
		},
	}

	pf := createMockPricingFetcher(t, mockClient)

	// Test that errors are handled gracefully
	err := pf.fetchFargateRates()
	if err == nil {
		t.Error("Expected error when API returns throttling exception")
	}

	// Test getting cached pricing when no data exists
	_, err = pf.GetFargateRates("us-east-1")
	if err == nil {
		t.Error("Expected error when no cached pricing data exists")
	}
}

func TestPricingFetcher_RegionMapping(t *testing.T) {
	pf := &PricingFetcher{}

	testCases := []struct {
		region   string
		expected string
	}{
		{"us-east-1", "US East (N. Virginia)"},
		{"us-west-2", "US West (Oregon)"},
		{"eu-west-1", "Europe (Ireland)"},
		{"ap-southeast-1", "Asia Pacific (Singapore)"},
		{"unknown-region", "unknown-region"}, // Fallback
	}

	for _, tc := range testCases {
		result := pf.getLocationName(tc.region)
		if result != tc.expected {
			t.Errorf("getLocationName(%s) = %s, expected %s", tc.region, result, tc.expected)
		}
	}
}

func TestPricingFetcher_UsageTypePrefix(t *testing.T) {
	pf := &PricingFetcher{}

	testCases := []struct {
		region   string
		expected string
	}{
		{"us-east-1", "USE1"},
		{"us-west-2", "USW2"},
		{"eu-west-1", "EUW1"},
		{"ap-southeast-1", "APS1"},
		{"invalid", "INVALID"}, // Fallback
	}

	for _, tc := range testCases {
		result := pf.getUsageTypePrefix(tc.region)
		if result != tc.expected {
			t.Errorf("getUsageTypePrefix(%s) = %s, expected %s", tc.region, result, tc.expected)
		}
	}
}

// Benchmark tests for performance validation
func BenchmarkPricingFetcher_ParseFargatePricing(b *testing.B) {
	pf := &PricingFetcher{}
	mockProduct := `{
		"product": {
			"attributes": {
				"vcpu": "0.25",
				"memory": "0.5 GB"
			}
		},
		"terms": {
			"OnDemand": {
				"term1": {
					"priceDimensions": {
						"dimension1": {
							"pricePerUnit": {
								"USD": "0.04048"
							}
						}
					}
				}
			}
		}
	}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pf.parseFargatePricing(mockProduct)
		if err != nil {
			b.Errorf("parseFargatePricing() failed: %v", err)
		}
	}
}

func BenchmarkPricingFetcher_GetLocationName(b *testing.B) {
	pf := &PricingFetcher{}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pf.getLocationName("us-east-1")
	}
}