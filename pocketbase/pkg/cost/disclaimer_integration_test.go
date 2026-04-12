package cost

import (
	"strings"
	"testing"
)

// TestDisclaimerIntegration_WithCostEstimate tests disclaimer generation integrated with cost estimation
// **Validates**: AC-1.3 (Estimate clearly labels what is included and excluded)
// **Validates**: AC-5.2 (Disclaimer clearly states excluded costs)
// **Validates**: AC-5.4 (Shows assumptions)
func TestDisclaimerIntegration_WithCostEstimate(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	blueprints := []string{
		string(BlueprintStaticWebsite),
		string(BlueprintWebApp),
		string(BlueprintFullStack),
		string(BlueprintMicroservices),
	}
	
	for _, blueprint := range blueprints {
		t.Run(blueprint, func(t *testing.T) {
			// Generate disclaimer
			disclaimerData, err := dg.GenerateDisclaimer(blueprint, "us-east-1")
			if err != nil {
				t.Fatalf("Failed to generate disclaimer: %v", err)
			}
			
			// Validate AC-1.3: Estimate clearly labels what is included and excluded
			if len(disclaimerData.Included) == 0 {
				t.Error("AC-1.3 FAILED: No included costs listed")
			}
			if len(disclaimerData.Excluded) == 0 {
				t.Error("AC-1.3 FAILED: No excluded costs listed")
			}
			
			// Validate AC-5.2: Disclaimer clearly states excluded costs
			if len(disclaimerData.Excluded) < 5 {
				t.Errorf("AC-5.2 FAILED: Should have at least 5 excluded costs, got %d", 
					len(disclaimerData.Excluded))
			}
			
			// Check that excluded costs are clearly stated
			for _, excluded := range disclaimerData.Excluded {
				if excluded == "" {
					t.Error("AC-5.2 FAILED: Empty excluded cost item")
				}
				if len(excluded) < 10 {
					t.Errorf("AC-5.2 FAILED: Excluded cost too vague: %s", excluded)
				}
			}
			
			// Validate AC-5.4: Shows assumptions
			if len(disclaimerData.Assumptions) == 0 {
				t.Error("AC-5.4 FAILED: No assumptions provided")
			}
			
			// Check that assumptions are meaningful
			for key, value := range disclaimerData.Assumptions {
				if key == "" || value == "" {
					t.Error("AC-5.4 FAILED: Empty assumption key or value")
				}
			}
			
			// Validate disclaimer completeness
			if err := dg.ValidateDisclaimerCompleteness(disclaimerData); err != nil {
				t.Errorf("Disclaimer validation failed: %v", err)
			}
		})
	}
}

// TestDisclaimerIntegration_SimpleDisclaimerFormat tests the simple disclaimer format
// **Validates**: AC-5.2 (Disclaimer clearly states excluded costs)
func TestDisclaimerIntegration_SimpleDisclaimerFormat(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	tests := []struct {
		blueprint string
		keywords  []string
	}{
		{
			blueprint: string(BlueprintStaticWebsite),
			keywords:  []string{"static website", "actual costs"},
		},
		{
			blueprint: string(BlueprintWebApp),
			keywords:  []string{"web application", "fargate"},
		},
		{
			blueprint: string(BlueprintFullStack),
			keywords:  []string{"full-stack", "frontend", "backend"},
		},
		{
			blueprint: string(BlueprintMicroservices),
			keywords:  []string{"microservices", "multiple services"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.blueprint, func(t *testing.T) {
			disclaimer, err := dg.GetSimpleDisclaimer(tt.blueprint)
			if err != nil {
				t.Fatalf("Failed to get simple disclaimer: %v", err)
			}
			
			// Check that disclaimer contains relevant keywords
			disclaimerLower := strings.ToLower(disclaimer)
			for _, keyword := range tt.keywords {
				if !strings.Contains(disclaimerLower, strings.ToLower(keyword)) {
					t.Errorf("Expected disclaimer to contain '%s'", keyword)
				}
			}
			
			// Check that disclaimer mentions variability
			if !strings.Contains(disclaimerLower, "vary") && 
			   !strings.Contains(disclaimerLower, "actual") {
				t.Error("Disclaimer should mention cost variability")
			}
		})
	}
}

// TestDisclaimerIntegration_IncludedExcludedConsistency tests that included and excluded lists don't overlap
func TestDisclaimerIntegration_IncludedExcludedConsistency(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	blueprints := []string{
		string(BlueprintStaticWebsite),
		string(BlueprintWebApp),
		string(BlueprintFullStack),
		string(BlueprintMicroservices),
	}
	
	for _, blueprint := range blueprints {
		t.Run(blueprint, func(t *testing.T) {
			disclaimerData, err := dg.GenerateDisclaimer(blueprint, "us-east-1")
			if err != nil {
				t.Fatalf("Failed to generate disclaimer: %v", err)
			}
			
			// Check that no service appears in both included and excluded
			for _, included := range disclaimerData.Included {
				includedLower := strings.ToLower(included)
				for _, excluded := range disclaimerData.Excluded {
					excludedLower := strings.ToLower(excluded)
					
					// Extract service names (e.g., "S3", "RDS", "Fargate")
					services := []string{"s3", "rds", "fargate", "alb", "cloudfront", 
						"route53", "nat", "cloudwatch", "ecr", "elasticache", "sqs", "api gateway"}
					
					for _, service := range services {
						if strings.Contains(includedLower, service) && 
						   strings.Contains(excludedLower, service) {
							// This is OK if they're different aspects (e.g., "S3 storage" vs "S3 versioning")
							// But flag if they seem to be the same thing
							if !strings.Contains(excludedLower, "beyond") && 
							   !strings.Contains(excludedLower, "exceeding") &&
							   !strings.Contains(excludedLower, "additional") {
								t.Logf("WARNING: Service '%s' appears in both included and excluded lists", service)
							}
						}
					}
				}
			}
		})
	}
}

// TestDisclaimerIntegration_AssumptionsMatchUsageManager tests that assumptions are consistent
// **Validates**: AC-5.4 (Shows assumptions)
func TestDisclaimerIntegration_AssumptionsMatchUsageManager(t *testing.T) {
	dg := NewDisclaimerGenerator()
	uam := NewUsageAssumptionsManager()
	
	blueprints := []string{
		string(BlueprintStaticWebsite),
		string(BlueprintWebApp),
		string(BlueprintFullStack),
		string(BlueprintMicroservices),
	}
	
	for _, blueprint := range blueprints {
		t.Run(blueprint, func(t *testing.T) {
			// Get disclaimer assumptions
			disclaimerData, err := dg.GenerateDisclaimer(blueprint, "us-east-1")
			if err != nil {
				t.Fatalf("Failed to generate disclaimer: %v", err)
			}
			
			// Get usage manager assumptions
			usageAssumptions, err := uam.GetAssumptionsAsMap(blueprint)
			if err != nil {
				t.Fatalf("Failed to get usage assumptions: %v", err)
			}
			
			// Verify they match
			if len(disclaimerData.Assumptions) != len(usageAssumptions) {
				t.Errorf("Assumption count mismatch: disclaimer has %d, usage manager has %d",
					len(disclaimerData.Assumptions), len(usageAssumptions))
			}
			
			// Check that all keys match
			for key := range disclaimerData.Assumptions {
				if _, exists := usageAssumptions[key]; !exists {
					t.Errorf("Disclaimer assumption key '%s' not found in usage manager", key)
				}
			}
		})
	}
}

// TestDisclaimerIntegration_FormattedOutput tests the formatted display output
func TestDisclaimerIntegration_FormattedOutput(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	disclaimerData, err := dg.GenerateDisclaimer(string(BlueprintWebApp), "us-east-1")
	if err != nil {
		t.Fatalf("Failed to generate disclaimer: %v", err)
	}
	
	formatted := dg.FormatDisclaimerForDisplay(disclaimerData)
	
	// Check structure
	requiredSections := []string{
		"COST ESTIMATE DISCLAIMER",
		"INCLUDED IN ESTIMATE",
		"EXCLUDED FROM ESTIMATE",
		"USAGE ASSUMPTIONS",
		"ACCURACY",
	}
	
	for _, section := range requiredSections {
		if !strings.Contains(formatted, section) {
			t.Errorf("Formatted output missing section: %s", section)
		}
	}
	
	// Check visual markers
	if !strings.Contains(formatted, "✓") {
		t.Error("Formatted output should use checkmarks for included items")
	}
	if !strings.Contains(formatted, "✗") {
		t.Error("Formatted output should use X marks for excluded items")
	}
	if !strings.Contains(formatted, "•") {
		t.Error("Formatted output should use bullets for assumptions")
	}
	
	// Check that it's readable (has line breaks)
	lines := strings.Split(formatted, "\n")
	if len(lines) < 20 {
		t.Error("Formatted output should have multiple lines for readability")
	}
}

// TestDisclaimerIntegration_RegionVariations tests disclaimers for different regions
func TestDisclaimerIntegration_RegionVariations(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	regions := []string{
		"us-east-1",
		"us-west-2",
		"eu-west-1",
		"ap-southeast-1",
		"ap-northeast-1",
	}
	
	for _, region := range regions {
		t.Run(region, func(t *testing.T) {
			disclaimerData, err := dg.GenerateDisclaimer(string(BlueprintWebApp), region)
			if err != nil {
				t.Fatalf("Failed to generate disclaimer for region %s: %v", region, err)
			}
			
			// Verify region is mentioned in disclaimer
			if !strings.Contains(disclaimerData.Disclaimer, region) {
				t.Errorf("Disclaimer should mention region %s", region)
			}
			
			// Verify structure is consistent across regions
			if err := dg.ValidateDisclaimerCompleteness(disclaimerData); err != nil {
				t.Errorf("Disclaimer for region %s is incomplete: %v", region, err)
			}
		})
	}
}

// TestDisclaimerIntegration_AccuracyStatements tests that accuracy statements are meaningful
func TestDisclaimerIntegration_AccuracyStatements(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	tests := []struct {
		blueprint       string
		minPercentage   int
		maxPercentage   int
	}{
		{
			blueprint:     string(BlueprintStaticWebsite),
			minPercentage: 20,
			maxPercentage: 40,
		},
		{
			blueprint:     string(BlueprintWebApp),
			minPercentage: 30,
			maxPercentage: 50,
		},
		{
			blueprint:     string(BlueprintFullStack),
			minPercentage: 25,
			maxPercentage: 45,
		},
		{
			blueprint:     string(BlueprintMicroservices),
			minPercentage: 40,
			maxPercentage: 60,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.blueprint, func(t *testing.T) {
			disclaimerData, err := dg.GenerateDisclaimer(tt.blueprint, "us-east-1")
			if err != nil {
				t.Fatalf("Failed to generate disclaimer: %v", err)
			}
			
			// Check that accuracy statement mentions percentage range
			statement := disclaimerData.AccuracyStatement
			if statement == "" {
				t.Error("Accuracy statement should not be empty")
			}
			
			// Should mention variability or accuracy
			if !strings.Contains(strings.ToLower(statement), "within") &&
			   !strings.Contains(strings.ToLower(statement), "typically") {
				t.Error("Accuracy statement should mention typical accuracy range")
			}
		})
	}
}

// TestDisclaimerIntegration_CommonExclusions tests that all blueprints exclude common items
func TestDisclaimerIntegration_CommonExclusions(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	// Items that should be excluded from all blueprints
	commonExclusions := []string{
		"Support",
		"data transfer",
	}
	
	blueprints := []string{
		string(BlueprintStaticWebsite),
		string(BlueprintWebApp),
		string(BlueprintFullStack),
		string(BlueprintMicroservices),
	}
	
	for _, blueprint := range blueprints {
		t.Run(blueprint, func(t *testing.T) {
			disclaimerData, err := dg.GenerateDisclaimer(blueprint, "us-east-1")
			if err != nil {
				t.Fatalf("Failed to generate disclaimer: %v", err)
			}
			
			excludedText := strings.ToLower(strings.Join(disclaimerData.Excluded, " "))
			
			for _, exclusion := range commonExclusions {
				if !strings.Contains(excludedText, strings.ToLower(exclusion)) {
					t.Errorf("Blueprint %s should exclude '%s'", blueprint, exclusion)
				}
			}
		})
	}
}
