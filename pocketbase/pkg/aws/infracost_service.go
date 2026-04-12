package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type InfracostService struct {
	apiKey     string
	client     *http.Client
	apiBaseURL string
}

type InfracostRequest struct {
	Path   string `json:"path"`
	Region string `json:"region,omitempty"`
}

type InfracostResponse struct {
	Version          string             `json:"version"`
	Currency         string             `json:"currency"`
	TotalMonthlyCost string             `json:"totalMonthlyCost"`
	TotalHourlyCost  string             `json:"totalHourlyCost"`
	Projects         []InfracostProject `json:"projects"`
}

type InfracostProject struct {
	Name                 string             `json:"name"`
	Metadata             map[string]string  `json:"metadata"`
	PastBreakdown        InfracostBreakdown `json:"pastBreakdown"`
	Breakdown            InfracostBreakdown `json:"breakdown"`
	Diff                 InfracostBreakdown `json:"diff"`
	Summary              InfracostSummary   `json:"summary"`
}

type InfracostBreakdown struct {
	Resources      []InfracostResource `json:"resources"`
	TotalMonthlyCost string            `json:"totalMonthlyCost"`
	TotalHourlyCost  string            `json:"totalHourlyCost"`
}

type InfracostResource struct {
	Name            string                   `json:"name"`
	ResourceType    string                   `json:"resourceType"`
	Tags            map[string]string        `json:"tags"`
	MonthlyCost     string                   `json:"monthlyCost"`
	HourlyCost      string                   `json:"hourlyCost"`
	CostComponents  []InfracostCostComponent `json:"costComponents"`
}

type InfracostCostComponent struct {
	Name            string `json:"name"`
	Unit            string `json:"unit"`
	HourlyQuantity  string `json:"hourlyQuantity"`
	MonthlyQuantity string `json:"monthlyQuantity"`
	Price           string `json:"price"`
	HourlyCost      string `json:"hourlyCost"`
	MonthlyCost     string `json:"monthlyCost"`
}

type InfracostSummary struct {
	TotalDetectedResources    int    `json:"totalDetectedResources"`
	TotalSupportedResources   int    `json:"totalSupportedResources"`
	TotalUnsupportedResources int    `json:"totalUnsupportedResources"`
	TotalUsageBasedResources  int    `json:"totalUsageBasedResources"`
	TotalNoPriceResources     int    `json:"totalNoPriceResources"`
	UnsupportedResourceCounts map[string]int `json:"unsupportedResourceCounts"`
	NoPriceResourceCounts     map[string]int `json:"noPriceResourceCounts"`
}

func NewInfracostService(apiKey string) *InfracostService {
	return &InfracostService{
		apiKey:     apiKey,
		apiBaseURL: "https://pricing.api.infracost.io",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// EstimateCost estimates the cost of Terraform code using Infracost API
func (s *InfracostService) EstimateCost(ctx context.Context, terraformCode, region string) (*CostEstimate, error) {
	// Prepare request body
	reqBody := InfracostRequest{
		Path:   terraformCode,
		Region: region,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/breakdown", s.apiBaseURL),
		bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.apiKey)

	// Execute request
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Infracost API: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Infracost API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var infracostResp InfracostResponse
	if err := json.Unmarshal(body, &infracostResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to our CostEstimate format
	return s.convertToCostEstimate(infracostResp, region)
}

// convertToCostEstimate converts Infracost response to our CostEstimate format
func (s *InfracostService) convertToCostEstimate(resp InfracostResponse, region string) (*CostEstimate, error) {
	// Parse total monthly cost
	var totalCost float64
	if resp.TotalMonthlyCost != "" {
		_, err := fmt.Sscanf(resp.TotalMonthlyCost, "%f", &totalCost)
		if err != nil {
			return nil, fmt.Errorf("failed to parse total cost: %w", err)
		}
	}

	// Build breakdown by resource
	breakdown := make(map[string]float64)
	if len(resp.Projects) > 0 {
		for _, resource := range resp.Projects[0].Breakdown.Resources {
			var cost float64
			if resource.MonthlyCost != "" {
				fmt.Sscanf(resource.MonthlyCost, "%f", &cost)
			}
			breakdown[resource.Name] = cost
		}
	}

	return &CostEstimate{
		TotalMonthlyCost: totalCost,
		Breakdown:        breakdown,
		Currency:         resp.Currency,
		Region:           region,
		Disclaimer:       "Estimate provided by Infracost. Actual costs may vary based on usage patterns and AWS pricing changes.",
	}, nil
}

// ValidateAPIKey validates the Infracost API key
func (s *InfracostService) ValidateAPIKey(ctx context.Context) error {
	// Simple test request to validate API key
	testTerraform := `
provider "aws" {
  region = "us-east-1"
}

resource "aws_instance" "test" {
  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t2.micro"
}
`

	_, err := s.EstimateCost(ctx, testTerraform, "us-east-1")
	return err
}
