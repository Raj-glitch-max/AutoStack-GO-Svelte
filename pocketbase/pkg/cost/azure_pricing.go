package cost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AzurePricingService fetches pricing from the Azure Retail Prices API.
// Docs: https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices
// No authentication required — public API.
type AzurePricingService struct {
	httpClient *http.Client
}

// AzureServicePricing holds per-unit prices for Azure services
type AzureServicePricing struct {
	ContainerAppsVCPUPerHour   float64 // per vCPU-hour
	ContainerAppsMemoryPerHour float64 // per GB-hour
	ContainerAppsRequestsPer1M float64 // per 1M requests
	SQLDatabasePerHour         float64 // S0 tier per hour
	BlobStoragePerGBMonth      float64 // LRS hot tier per GB/month
	CDNEgressPerGB             float64 // Standard Microsoft CDN per GB
	FetchedAt                  time.Time
}

// NewAzurePricingService creates a new Azure pricing service
func NewAzurePricingService() *AzurePricingService {
	return &AzurePricingService{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// GetPricing returns current Azure pricing for the given region.
// Falls back to well-known defaults if the API is unavailable.
func (a *AzurePricingService) GetPricing(ctx context.Context, region string) (*AzureServicePricing, error) {
	pricing, err := a.fetchFromAPI(ctx, region)
	if err != nil {
		return a.defaultPricing(), nil
	}
	return pricing, nil
}

// fetchFromAPI calls the Azure Retail Prices API
func (a *AzurePricingService) fetchFromAPI(ctx context.Context, region string) (*AzureServicePricing, error) {
	// Normalize region name for Azure API (e.g., "eastus" → "East US")
	azureRegion := normalizeAzureRegion(region)

	filter := fmt.Sprintf(
		"armRegionName eq '%s' and priceType eq 'Consumption' and (serviceName eq 'Azure Container Apps' or serviceName eq 'SQL Database' or serviceName eq 'Storage' or serviceName eq 'Content Delivery Network')",
		azureRegion,
	)
	url := "https://prices.azure.com/api/retail/prices?$filter=" + strings.ReplaceAll(filter, " ", "%20")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Azure pricing API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Azure pricing API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Items []struct {
			SkuName      string  `json:"skuName"`
			ProductName  string  `json:"productName"`
			ServiceName  string  `json:"serviceName"`
			RetailPrice  float64 `json:"retailPrice"`
			UnitOfMeasure string `json:"unitOfMeasure"`
		} `json:"Items"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	pricing := a.defaultPricing()
	pricing.FetchedAt = time.Now()

	for _, item := range result.Items {
		switch {
		case item.ServiceName == "Azure Container Apps" && strings.Contains(item.SkuName, "vCPU"):
			pricing.ContainerAppsVCPUPerHour = item.RetailPrice
		case item.ServiceName == "Azure Container Apps" && strings.Contains(item.SkuName, "Memory"):
			pricing.ContainerAppsMemoryPerHour = item.RetailPrice
		case item.ServiceName == "Azure Container Apps" && strings.Contains(item.SkuName, "Request"):
			pricing.ContainerAppsRequestsPer1M = item.RetailPrice
		case item.ServiceName == "SQL Database" && strings.Contains(item.SkuName, "S0"):
			pricing.SQLDatabasePerHour = item.RetailPrice
		case item.ServiceName == "Storage" && strings.Contains(item.ProductName, "Blob") && strings.Contains(item.SkuName, "Hot"):
			pricing.BlobStoragePerGBMonth = item.RetailPrice
		case item.ServiceName == "Content Delivery Network" && strings.Contains(item.SkuName, "Standard"):
			pricing.CDNEgressPerGB = item.RetailPrice
		}
	}

	return pricing, nil
}

// defaultPricing returns conservative Azure pricing defaults (as of 2025)
func (a *AzurePricingService) defaultPricing() *AzureServicePricing {
	return &AzureServicePricing{
		ContainerAppsVCPUPerHour:   0.000024, // $0.000024/vCPU-second
		ContainerAppsMemoryPerHour: 0.000003, // $0.000003/GB-second
		ContainerAppsRequestsPer1M: 0.40,     // $0.40 per 1M requests
		SQLDatabasePerHour:         0.0202,   // S0 tier ~$0.0202/hr
		BlobStoragePerGBMonth:      0.018,    // LRS hot $0.018/GB/month
		CDNEgressPerGB:             0.087,    // Standard Microsoft CDN
		FetchedAt:                  time.Now(),
	}
}

// EstimateContainerAppsMonthly estimates monthly cost for Azure Container Apps
func (a *AzurePricingService) EstimateContainerAppsMonthly(pricing *AzureServicePricing, vcpu float64, memoryGB float64, requestsPerMonth int64) float64 {
	hoursPerMonth := 730.0
	utilizationFactor := 0.5 // scales to zero

	cpuCost := vcpu * hoursPerMonth * utilizationFactor * pricing.ContainerAppsVCPUPerHour
	memoryCost := memoryGB * hoursPerMonth * utilizationFactor * pricing.ContainerAppsMemoryPerHour
	requestCost := float64(requestsPerMonth) / 1_000_000 * pricing.ContainerAppsRequestsPer1M

	return cpuCost + memoryCost + requestCost
}

// EstimateAzureSQLMonthly estimates monthly cost for Azure SQL Database
func (a *AzurePricingService) EstimateAzureSQLMonthly(pricing *AzureServicePricing) float64 {
	return pricing.SQLDatabasePerHour * 730.0
}

// EstimateBlobStorageMonthly estimates monthly cost for Blob Storage + CDN
func (a *AzurePricingService) EstimateBlobStorageMonthly(pricing *AzureServicePricing, storageGB float64, egressGB float64) float64 {
	storageCost := storageGB * pricing.BlobStoragePerGBMonth
	egressCost := egressGB * pricing.CDNEgressPerGB
	return storageCost + egressCost
}

// AzureBlueprintEstimate returns a cost estimate for an Azure blueprint
func AzureBlueprintEstimate(blueprintID string, region string) (min, estimate, max float64) {
	svc := NewAzurePricingService()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pricing, _ := svc.GetPricing(ctx, region)

	switch blueprintID {
	case "azure-container-apps":
		est := svc.EstimateContainerAppsMonthly(pricing, 0.5, 1.0, 1_000_000)
		return est * 0.7, est, est * 1.4

	case "azure-full-stack":
		appCost := svc.EstimateContainerAppsMonthly(pricing, 0.5, 1.0, 1_000_000)
		sqlCost := svc.EstimateAzureSQLMonthly(pricing)
		est := appCost + sqlCost
		return est * 0.8, est, est * 1.4

	case "azure-static-site":
		est := svc.EstimateBlobStorageMonthly(pricing, 5.0, 10.0)
		return est * 0.7, est, est * 1.5

	default:
		return 5, 10, 20
	}
}

// normalizeAzureRegion converts short region names to Azure display names
func normalizeAzureRegion(region string) string {
	m := map[string]string{
		"eastus":        "East US",
		"eastus2":       "East US 2",
		"westus":        "West US",
		"westus2":       "West US 2",
		"westus3":       "West US 3",
		"northeurope":   "North Europe",
		"westeurope":    "West Europe",
		"uksouth":       "UK South",
		"eastasia":      "East Asia",
		"southeastasia": "Southeast Asia",
	}
	if v, ok := m[region]; ok {
		return v
	}
	return region
}
