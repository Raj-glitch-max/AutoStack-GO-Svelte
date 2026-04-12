package cost

import (
	"fmt"
	"strings"
)

// DisclaimerGenerator generates cost disclaimers and usage assumptions
// **Validates**: AC-1.3 (Estimate clearly labels what is included and excluded)
// **Validates**: AC-5.2 (Disclaimer clearly states excluded costs)
// **Validates**: AC-5.4 (Shows assumptions)
type DisclaimerGenerator struct {
	assumptionsManager *UsageAssumptionsManager
}

// NewDisclaimerGenerator creates a new disclaimer generator
func NewDisclaimerGenerator() *DisclaimerGenerator {
	return &DisclaimerGenerator{
		assumptionsManager: NewUsageAssumptionsManager(),
	}
}

// DisclaimerData contains comprehensive disclaimer information
type DisclaimerData struct {
	// Main disclaimer text
	Disclaimer string `json:"disclaimer"`
	
	// What's included in the estimate
	Included []string `json:"included"`
	
	// What's excluded from the estimate
	Excluded []string `json:"excluded"`
	
	// Usage assumptions
	Assumptions map[string]string `json:"assumptions"`
	
	// Important notes and caveats
	Notes []string `json:"notes"`
	
	// Accuracy disclaimer
	AccuracyStatement string `json:"accuracyStatement"`
}

// GenerateDisclaimer generates a comprehensive disclaimer for a blueprint type
// **Validates**: AC-1.3 (Estimate clearly labels what is included and excluded)
func (dg *DisclaimerGenerator) GenerateDisclaimer(blueprintType string, region string) (*DisclaimerData, error) {
	// Get usage assumptions
	assumptions, err := dg.assumptionsManager.GetAssumptionsAsMap(blueprintType)
	if err != nil {
		return nil, fmt.Errorf("failed to get assumptions: %w", err)
	}

	// Generate blueprint-specific disclaimer
	var disclaimer DisclaimerData
	
	switch BlueprintType(blueprintType) {
	case BlueprintStaticWebsite:
		disclaimer = dg.generateStaticWebsiteDisclaimer(region, assumptions)
	case BlueprintWebApp:
		disclaimer = dg.generateWebAppDisclaimer(region, assumptions)
	case BlueprintFullStack:
		disclaimer = dg.generateFullStackDisclaimer(region, assumptions)
	case BlueprintMicroservices:
		disclaimer = dg.generateMicroservicesDisclaimer(region, assumptions)
	default:
		return nil, fmt.Errorf("unsupported blueprint type: %s", blueprintType)
	}

	return &disclaimer, nil
}

// generateStaticWebsiteDisclaimer generates disclaimer for static website blueprint
func (dg *DisclaimerGenerator) generateStaticWebsiteDisclaimer(region string, assumptions map[string]string) DisclaimerData {
	return DisclaimerData{
		Disclaimer: fmt.Sprintf(
			"This estimate is for a static website hosted in %s. "+
				"Actual costs may vary based on traffic patterns, content size, and geographic distribution of users. "+
				"Estimates are based on AWS pricing as of the last cache update and may not reflect recent price changes.",
			region,
		),
		Included: []string{
			"Amazon S3 storage costs for static assets",
			"Amazon S3 request costs (GET, PUT, LIST operations)",
			"Amazon CloudFront data transfer out to internet",
			"Amazon CloudFront request costs",
			"Amazon Route53 hosted zone (if custom domain enabled)",
		},
		Excluded: []string{
			"Data transfer costs exceeding assumed monthly limits",
			"S3 storage costs for backups or versioning",
			"CloudFront invalidation requests",
			"Route53 query charges beyond included quota",
			"AWS WAF or Shield Advanced protection",
			"CloudWatch detailed monitoring and custom metrics",
			"S3 Intelligent-Tiering or Glacier storage classes",
			"Cross-region data transfer",
			"AWS Support plan costs",
		},
		Assumptions: assumptions,
		Notes: []string{
			"Estimate assumes standard S3 storage class",
			"CloudFront pricing varies by geographic region of requests",
			"Cache hit ratio significantly impacts CloudFront costs",
			"Actual data transfer may vary based on content compression",
			"First 1 TB of data transfer out per month is typically most expensive",
		},
		AccuracyStatement: "Estimates are typically within 20-40% of actual costs for static websites. " +
			"Actual costs depend heavily on traffic patterns and content delivery efficiency.",
	}
}

// generateWebAppDisclaimer generates disclaimer for web application blueprint
func (dg *DisclaimerGenerator) generateWebAppDisclaimer(region string, assumptions map[string]string) DisclaimerData {
	return DisclaimerData{
		Disclaimer: fmt.Sprintf(
			"This estimate is for a containerized web application running on AWS Fargate in %s. "+
				"Actual costs may vary significantly based on application load, database usage, and scaling behavior. "+
				"This estimate assumes 24/7 operation with the specified resource configuration.",
			region,
		),
		Included: []string{
			"AWS Fargate vCPU and memory costs (24/7 operation)",
			"Application Load Balancer hourly costs",
			"Application Load Balancer LCU (Load Balancer Capacity Units) costs",
			"Amazon RDS instance costs (24/7 operation)",
			"Amazon RDS storage costs (General Purpose SSD)",
			"NAT Gateway hourly costs and data processing",
			"CloudWatch Logs ingestion and storage",
			"Amazon ECR storage for container images",
		},
		Excluded: []string{
			"Auto-scaling costs beyond base task count",
			"RDS backup storage beyond retention period",
			"RDS data transfer costs",
			"RDS IOPS beyond General Purpose SSD baseline",
			"RDS Multi-AZ deployment costs",
			"RDS read replicas",
			"CloudWatch alarms and custom metrics",
			"CloudWatch Logs data transfer and exports",
			"ECR data transfer costs",
			"VPC endpoint costs",
			"Secrets Manager or Parameter Store costs",
			"AWS Certificate Manager (ACM) is free for public certificates",
			"Data transfer between availability zones",
			"AWS Support plan costs",
		},
		Assumptions: assumptions,
		Notes: []string{
			"Estimate assumes single availability zone deployment",
			"RDS costs assume General Purpose SSD (gp2) storage",
			"NAT Gateway costs can be reduced with VPC endpoints",
			"CloudWatch Logs costs depend on log verbosity",
			"Fargate pricing varies by vCPU and memory combination",
			"ALB LCU costs depend on request rate, connections, and bandwidth",
			"Database costs assume standard backup retention (7 days)",
		},
		AccuracyStatement: "Estimates are typically within 30-50% of actual costs for web applications. " +
			"Actual costs vary significantly based on traffic patterns, database query efficiency, and auto-scaling behavior.",
	}
}

// generateFullStackDisclaimer generates disclaimer for full-stack application blueprint
func (dg *DisclaimerGenerator) generateFullStackDisclaimer(region string, assumptions map[string]string) DisclaimerData {
	return DisclaimerData{
		Disclaimer: fmt.Sprintf(
			"This estimate is for a full-stack application with separate frontend (S3/CloudFront) and backend (Fargate) in %s. "+
				"Actual costs may vary based on user traffic, API usage patterns, and database workload. "+
				"This estimate assumes 24/7 backend operation with moderate frontend traffic.",
			region,
		),
		Included: []string{
			"Amazon S3 storage for frontend static assets",
			"Amazon CloudFront data transfer and requests for frontend",
			"AWS Fargate vCPU and memory costs for backend (24/7)",
			"Application Load Balancer costs (hourly + LCUs)",
			"Amazon RDS instance and storage costs (24/7)",
			"Amazon Route53 hosted zone for custom domain",
			"NAT Gateway hourly costs and data processing",
			"CloudWatch Logs ingestion and storage",
			"Amazon ECR storage for backend container images",
		},
		Excluded: []string{
			"Backend auto-scaling costs beyond base task count",
			"Frontend assets exceeding assumed storage limits",
			"CloudFront invalidation requests",
			"RDS backup storage beyond retention period",
			"RDS Multi-AZ deployment costs",
			"RDS read replicas",
			"RDS IOPS beyond General Purpose SSD baseline",
			"CloudWatch alarms, dashboards, and custom metrics",
			"AWS WAF or Shield Advanced protection",
			"VPC endpoint costs",
			"Secrets Manager costs",
			"Data transfer between availability zones",
			"Cross-region data transfer",
			"AWS Support plan costs",
		},
		Assumptions: assumptions,
		Notes: []string{
			"Frontend costs scale with user traffic and content size",
			"Backend costs are relatively fixed (24/7 operation)",
			"Database costs assume single AZ deployment",
			"CloudFront pricing varies by geographic distribution of users",
			"NAT Gateway costs can be significant for high-traffic applications",
			"ALB costs depend on request rate and data processed",
			"Estimate assumes efficient caching strategy for frontend",
		},
		AccuracyStatement: "Estimates are typically within 25-45% of actual costs for full-stack applications. " +
			"Frontend costs are more predictable, while backend costs vary with load and scaling behavior.",
	}
}

// generateMicroservicesDisclaimer generates disclaimer for microservices blueprint
func (dg *DisclaimerGenerator) generateMicroservicesDisclaimer(region string, assumptions map[string]string) DisclaimerData {
	return DisclaimerData{
		Disclaimer: fmt.Sprintf(
			"This estimate is for a microservices architecture with multiple services, messaging, and caching in %s. "+
				"Actual costs may vary significantly based on service scaling, message volume, and cache utilization. "+
				"This is a complex architecture with many cost variables.",
			region,
		),
		Included: []string{
			"AWS Fargate costs for multiple microservices (24/7)",
			"Application Load Balancer costs (hourly + LCUs)",
			"Amazon RDS instance and storage costs (24/7)",
			"Amazon S3 storage for application data",
			"Amazon CloudFront data transfer and requests",
			"API Gateway request costs",
			"Amazon SQS message requests",
			"Amazon ElastiCache node costs (24/7)",
			"NAT Gateway hourly costs and data processing",
			"CloudWatch Logs ingestion and storage",
			"Amazon ECR storage for all service container images",
		},
		Excluded: []string{
			"Auto-scaling costs beyond base service count",
			"API Gateway data transfer costs",
			"SQS message storage beyond standard retention",
			"ElastiCache backup and snapshot costs",
			"ElastiCache Multi-AZ deployment costs",
			"RDS backup storage and Multi-AZ costs",
			"RDS read replicas",
			"Lambda functions (if used for event processing)",
			"Step Functions or EventBridge costs",
			"CloudWatch alarms, dashboards, and custom metrics",
			"AWS X-Ray tracing costs",
			"VPC endpoint costs",
			"Secrets Manager costs",
			"Data transfer between services and AZs",
			"Cross-region data transfer",
			"AWS Support plan costs",
		},
		Assumptions: assumptions,
		Notes: []string{
			"Microservices costs scale with number of services and instances",
			"Message queue costs depend on message size and retention",
			"Cache costs are relatively fixed (node-based pricing)",
			"API Gateway costs can be significant for high request volumes",
			"Inter-service communication generates data transfer costs",
			"NAT Gateway costs increase with service count",
			"CloudWatch Logs costs scale with service count and verbosity",
			"Consider VPC endpoints to reduce NAT Gateway costs",
		},
		AccuracyStatement: "Estimates are typically within 40-60% of actual costs for microservices architectures. " +
			"This is the most complex architecture with highest cost variability. Actual costs depend heavily on " +
			"service scaling, message patterns, and inter-service communication.",
	}
}

// GetSimpleDisclaimer returns a simple one-line disclaimer for a blueprint
// **Validates**: AC-5.2 (Disclaimer clearly states excluded costs)
func (dg *DisclaimerGenerator) GetSimpleDisclaimer(blueprintType string) (string, error) {
	disclaimerData, err := dg.GenerateDisclaimer(blueprintType, "us-east-1")
	if err != nil {
		return "", err
	}
	return disclaimerData.Disclaimer, nil
}

// FormatDisclaimerForDisplay formats disclaimer data for user-friendly display
func (dg *DisclaimerGenerator) FormatDisclaimerForDisplay(data *DisclaimerData) string {
	var builder strings.Builder
	
	builder.WriteString("COST ESTIMATE DISCLAIMER\n")
	builder.WriteString(strings.Repeat("=", 80))
	builder.WriteString("\n\n")
	
	builder.WriteString(data.Disclaimer)
	builder.WriteString("\n\n")
	
	builder.WriteString("INCLUDED IN ESTIMATE:\n")
	for _, item := range data.Included {
		builder.WriteString(fmt.Sprintf("  ✓ %s\n", item))
	}
	builder.WriteString("\n")
	
	builder.WriteString("EXCLUDED FROM ESTIMATE:\n")
	for _, item := range data.Excluded {
		builder.WriteString(fmt.Sprintf("  ✗ %s\n", item))
	}
	builder.WriteString("\n")
	
	builder.WriteString("USAGE ASSUMPTIONS:\n")
	for key, value := range data.Assumptions {
		builder.WriteString(fmt.Sprintf("  • %s: %s\n", key, value))
	}
	builder.WriteString("\n")
	
	if len(data.Notes) > 0 {
		builder.WriteString("IMPORTANT NOTES:\n")
		for _, note := range data.Notes {
			builder.WriteString(fmt.Sprintf("  ℹ %s\n", note))
		}
		builder.WriteString("\n")
	}
	
	builder.WriteString("ACCURACY:\n")
	builder.WriteString(fmt.Sprintf("  %s\n", data.AccuracyStatement))
	
	return builder.String()
}

// GetExcludedCosts returns a list of excluded costs for a blueprint
// **Validates**: AC-5.2 (Disclaimer clearly states excluded costs)
func (dg *DisclaimerGenerator) GetExcludedCosts(blueprintType string) ([]string, error) {
	disclaimerData, err := dg.GenerateDisclaimer(blueprintType, "us-east-1")
	if err != nil {
		return nil, err
	}
	return disclaimerData.Excluded, nil
}

// GetIncludedCosts returns a list of included costs for a blueprint
// **Validates**: AC-1.3 (Estimate clearly labels what is included and excluded)
func (dg *DisclaimerGenerator) GetIncludedCosts(blueprintType string) ([]string, error) {
	disclaimerData, err := dg.GenerateDisclaimer(blueprintType, "us-east-1")
	if err != nil {
		return nil, err
	}
	return disclaimerData.Included, nil
}

// ValidateDisclaimerCompleteness validates that a disclaimer has all required fields
func (dg *DisclaimerGenerator) ValidateDisclaimerCompleteness(data *DisclaimerData) error {
	if data.Disclaimer == "" {
		return fmt.Errorf("disclaimer text is required")
	}
	if len(data.Included) == 0 {
		return fmt.Errorf("included costs list cannot be empty")
	}
	if len(data.Excluded) == 0 {
		return fmt.Errorf("excluded costs list cannot be empty")
	}
	if len(data.Assumptions) == 0 {
		return fmt.Errorf("assumptions cannot be empty")
	}
	if data.AccuracyStatement == "" {
		return fmt.Errorf("accuracy statement is required")
	}
	return nil
}
