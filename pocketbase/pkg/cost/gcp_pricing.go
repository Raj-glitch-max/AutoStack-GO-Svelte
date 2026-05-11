package cost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GCPPricingService fetches pricing from the GCP Cloud Billing Catalog API.
// Docs: https://cloud.google.com/billing/v1/how-tos/catalog-api
type GCPPricingService struct {
	httpClient *http.Client
}

// GCPServicePricing holds per-unit prices for a GCP service
type GCPServicePricing struct {
	CloudRunCPUPerVCPUHour    float64 // per vCPU-hour
	CloudRunMemoryPerGBHour   float64 // per GB-hour
	CloudRunRequestsPer1M     float64 // per 1M requests
	CloudSQLPerHour           float64 // db-f1-micro per hour
	GCSStoragePerGBMonth      float64 // standard storage per GB/month
	GCSOperationsPer10K       float64 // class A operations per 10K
	CDNEgressPerGB            float64 // Cloud CDN egress per GB
	FetchedAt                 time.Time
}

// NewGCPPricingService creates a new GCP pricing service
func NewGCPPricingService() *GCPPricingService {
	return &GCPPricingService{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// GetPricing returns current GCP pricing for the given region.
// Falls back to well-known defaults if the API is unavailable.
func (g *GCPPricingService) GetPricing(ctx context.Context, region string) (*GCPServicePricing, error) {
	pricing, err := g.fetchFromAPI(ctx, region)
	if err != nil {
		// Return conservative defaults so cost estimation still works
		return g.defaultPricing(), nil
	}
	return pricing, nil
}

// fetchFromAPI calls the GCP Cloud Billing Catalog API (public, no auth required for list)
func (g *GCPPricingService) fetchFromAPI(ctx context.Context, region string) (*GCPServicePricing, error) {
	// Cloud Run pricing service ID
	url := "https://cloudbilling.googleapis.com/v1/services/152E-C115-5142/skus?pageSize=100"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GCP billing API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GCP billing API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Skus []struct {
			Description    string `json:"description"`
			ServiceRegions []string `json:"serviceRegions"`
			PricingInfo    []struct {
				PricingExpression struct {
					TieredRates []struct {
						UnitPrice struct {
							Units string `json:"units"`
							Nanos int64  `json:"nanos"`
						} `json:"unitPrice"`
					} `json:"tieredRates"`
				} `json:"pricingExpression"`
			} `json:"pricingInfo"`
		} `json:"skus"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	pricing := g.defaultPricing()
	pricing.FetchedAt = time.Now()

	// Parse relevant SKUs
	for _, sku := range result.Skus {
		if len(sku.PricingInfo) == 0 || len(sku.PricingInfo[0].PricingExpression.TieredRates) == 0 {
			continue
		}

		// Check if SKU applies to our region
		regionMatch := false
		for _, r := range sku.ServiceRegions {
			if r == region || r == "global" {
				regionMatch = true
				break
			}
		}
		if !regionMatch {
			continue
		}

		rate := sku.PricingInfo[0].PricingExpression.TieredRates[0]
		price := float64(rate.UnitPrice.Nanos) / 1e9

		switch {
		case containsAll(sku.Description, "CPU", "Allocation Time"):
			pricing.CloudRunCPUPerVCPUHour = price
		case containsAll(sku.Description, "Memory", "Allocation Time"):
			pricing.CloudRunMemoryPerGBHour = price
		case containsAll(sku.Description, "Request"):
			pricing.CloudRunRequestsPer1M = price
		}
	}

	return pricing, nil
}

// defaultPricing returns conservative GCP pricing defaults (as of 2025)
func (g *GCPPricingService) defaultPricing() *GCPServicePricing {
	return &GCPServicePricing{
		CloudRunCPUPerVCPUHour:  0.00002400, // $0.000024/vCPU-second → /3600
		CloudRunMemoryPerGBHour: 0.00000250, // $0.0000025/GB-second → /3600
		CloudRunRequestsPer1M:   0.40,       // $0.40 per 1M requests
		CloudSQLPerHour:         0.0150,     // db-f1-micro ~$0.015/hr
		GCSStoragePerGBMonth:    0.020,      // Standard storage $0.020/GB/month
		GCSOperationsPer10K:     0.05,       // Class A operations
		CDNEgressPerGB:          0.08,       // Cloud CDN egress
		FetchedAt:               time.Now(),
	}
}

// EstimateCloudRunMonthly estimates monthly cost for a Cloud Run service
// Assumes: 1 vCPU, 512MB memory, 1M requests/month, 50% utilization
func (g *GCPPricingService) EstimateCloudRunMonthly(pricing *GCPServicePricing, vcpu float64, memoryGB float64, requestsPerMonth int64) float64 {
	hoursPerMonth := 730.0
	utilizationFactor := 0.5 // Cloud Run scales to zero, assume 50% active

	cpuCost := vcpu * hoursPerMonth * utilizationFactor * pricing.CloudRunCPUPerVCPUHour
	memoryCost := memoryGB * hoursPerMonth * utilizationFactor * pricing.CloudRunMemoryPerGBHour
	requestCost := float64(requestsPerMonth) / 1_000_000 * pricing.CloudRunRequestsPer1M

	return cpuCost + memoryCost + requestCost
}

// EstimateCloudSQLMonthly estimates monthly cost for a Cloud SQL instance
func (g *GCPPricingService) EstimateCloudSQLMonthly(pricing *GCPServicePricing, storageGB float64) float64 {
	hoursPerMonth := 730.0
	instanceCost := pricing.CloudSQLPerHour * hoursPerMonth
	storageCost := storageGB * pricing.GCSStoragePerGBMonth
	return instanceCost + storageCost
}

// EstimateGCSMonthly estimates monthly cost for GCS + CDN static site
func (g *GCPPricingService) EstimateGCSMonthly(pricing *GCPServicePricing, storageGB float64, egressGB float64) float64 {
	storageCost := storageGB * pricing.GCSStoragePerGBMonth
	egressCost := egressGB * pricing.CDNEgressPerGB
	return storageCost + egressCost
}

// GCPBlueprintEstimate returns a cost estimate for a GCP blueprint
func GCPBlueprintEstimate(blueprintID string, region string) (min, estimate, max float64) {
	svc := NewGCPPricingService()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pricing, _ := svc.GetPricing(ctx, region)

	switch blueprintID {
	case "gcp-cloud-run":
		// 1 vCPU, 512MB, 1M requests/month
		est := svc.EstimateCloudRunMonthly(pricing, 1.0, 0.5, 1_000_000)
		return est * 0.7, est, est * 1.4

	case "gcp-full-stack":
		// Cloud Run + Cloud SQL db-f1-micro + 20GB storage
		runCost := svc.EstimateCloudRunMonthly(pricing, 1.0, 0.5, 1_000_000)
		sqlCost := svc.EstimateCloudSQLMonthly(pricing, 20.0)
		est := runCost + sqlCost
		return est * 0.8, est, est * 1.4

	case "gcp-static-site":
		// 5GB storage, 10GB egress/month
		est := svc.EstimateGCSMonthly(pricing, 5.0, 10.0)
		return est * 0.7, est, est * 1.5

	default:
		return 5, 10, 20
	}
}
