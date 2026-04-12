package cost

import (
	"strings"
	"testing"
)

func TestNewDisclaimerGenerator(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	if dg == nil {
		t.Fatal("Expected non-nil disclaimer generator")
	}
	
	if dg.assumptionsManager == nil {
		t.Error("Expected assumptions manager to be initialized")
	}
}

func TestGenerateDisclaimer_StaticWebsite(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	data, err := dg.GenerateDisclaimer(string(BlueprintStaticWebsite), "us-east-1")
	if err != nil {
		t.Fatalf("Failed to generate disclaimer: %v", err)
	}
	
	// Validate disclaimer completeness
	if err := dg.ValidateDisclaimerCompleteness(data); err != nil {
		t.Errorf("Disclaimer validation failed: %v", err)
	}
	
	// Check main disclaimer text
	if data.Disclaimer == "" {
		t.Error("Expected non-empty disclaimer text")
	}
	if !strings.Contains(data.Disclaimer, "us-east-1") {
		t.Error("Expected disclaimer to mention region")
	}
	if !strings.Contains(data.Disclaimer, "static website") {
		t.Error("Expected disclaimer to mention static website")
	}
	
	// Check included costs
	expectedIncluded := []string{"S3", "CloudFront", "Route53"}
	for _, expected := range expectedIncluded {
		found := false
		for _, included := range data.Included {
			if strings.Contains(included, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected '%s' in included costs", expected)
		}
	}
	
	// Check excluded costs
	if len(data.Excluded) == 0 {
		t.Error("Expected non-empty excluded costs list")
	}
	
	// Check assumptions
	if len(data.Assumptions) == 0 {
		t.Error("Expected non-empty assumptions map")
	}
	
	// Check accuracy statement
	if data.AccuracyStatement == "" {
		t.Error("Expected non-empty accuracy statement")
	}
}

func TestGenerateDisclaimer_WebApp(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	data, err := dg.GenerateDisclaimer(string(BlueprintWebApp), "eu-west-1")
	if err != nil {
		t.Fatalf("Failed to generate disclaimer: %v", err)
	}
	
	// Validate disclaimer completeness
	if err := dg.ValidateDisclaimerCompleteness(data); err != nil {
		t.Errorf("Disclaimer validation failed: %v", err)
	}
	
	// Check included costs for web app
	expectedIncluded := []string{"Fargate", "Load Balancer", "RDS", "NAT Gateway", "CloudWatch"}
	for _, expected := range expectedIncluded {
		found := false
		for _, included := range data.Included {
			if strings.Contains(included, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected '%s' in included costs", expected)
		}
	}
	
	// Check excluded costs mention important items
	excludedText := strings.Join(data.Excluded, " ")
	if !strings.Contains(excludedText, "Auto-scaling") && !strings.Contains(excludedText, "scaling") {
		t.Error("Expected excluded costs to mention auto-scaling")
	}
	if !strings.Contains(excludedText, "Multi-AZ") {
		t.Error("Expected excluded costs to mention Multi-AZ")
	}
	
	// Check notes
	if len(data.Notes) == 0 {
		t.Error("Expected non-empty notes list")
	}
}

func TestGenerateDisclaimer_FullStack(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	data, err := dg.GenerateDisclaimer(string(BlueprintFullStack), "ap-southeast-1")
	if err != nil {
		t.Fatalf("Failed to generate disclaimer: %v", err)
	}
	
	// Validate disclaimer completeness
	if err := dg.ValidateDisclaimerCompleteness(data); err != nil {
		t.Errorf("Disclaimer validation failed: %v", err)
	}
	
	// Check that it includes both frontend and backend services
	includedText := strings.Join(data.Included, " ")
	if !strings.Contains(includedText, "S3") {
		t.Error("Expected S3 (frontend) in included costs")
	}
	if !strings.Contains(includedText, "Fargate") {
		t.Error("Expected Fargate (backend) in included costs")
	}
	if !strings.Contains(includedText, "CloudFront") {
		t.Error("Expected CloudFront (frontend CDN) in included costs")
	}
	
	// Check region in disclaimer
	if !strings.Contains(data.Disclaimer, "ap-southeast-1") {
		t.Error("Expected disclaimer to mention region")
	}
}

func TestGenerateDisclaimer_Microservices(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	data, err := dg.GenerateDisclaimer(string(BlueprintMicroservices), "us-west-2")
	if err != nil {
		t.Fatalf("Failed to generate disclaimer: %v", err)
	}
	
	// Validate disclaimer completeness
	if err := dg.ValidateDisclaimerCompleteness(data); err != nil {
		t.Errorf("Disclaimer validation failed: %v", err)
	}
	
	// Check that it includes microservices-specific services
	includedText := strings.Join(data.Included, " ")
	expectedServices := []string{"Fargate", "API Gateway", "SQS", "ElastiCache"}
	for _, service := range expectedServices {
		if !strings.Contains(includedText, service) {
			t.Errorf("Expected '%s' in included costs for microservices", service)
		}
	}
	
	// Check that disclaimer mentions complexity
	if !strings.Contains(data.Disclaimer, "microservices") {
		t.Error("Expected disclaimer to mention microservices")
	}
	
	// Check accuracy statement mentions variability
	if !strings.Contains(data.AccuracyStatement, "variability") && 
	   !strings.Contains(data.AccuracyStatement, "vary") {
		t.Error("Expected accuracy statement to mention cost variability")
	}
}

func TestGenerateDisclaimer_UnsupportedBlueprint(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	_, err := dg.GenerateDisclaimer("invalid-blueprint", "us-east-1")
	if err == nil {
		t.Error("Expected error for unsupported blueprint type")
	}
	
	if !strings.Contains(err.Error(), "failed to get assumptions") && 
	   !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("Expected error about unsupported blueprint, got: %v", err)
	}
}

func TestGetSimpleDisclaimer(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	tests := []struct {
		name          string
		blueprintType string
		wantError     bool
	}{
		{
			name:          "Static Website",
			blueprintType: string(BlueprintStaticWebsite),
			wantError:     false,
		},
		{
			name:          "Web App",
			blueprintType: string(BlueprintWebApp),
			wantError:     false,
		},
		{
			name:          "Full Stack",
			blueprintType: string(BlueprintFullStack),
			wantError:     false,
		},
		{
			name:          "Microservices",
			blueprintType: string(BlueprintMicroservices),
			wantError:     false,
		},
		{
			name:          "Invalid Blueprint",
			blueprintType: "invalid",
			wantError:     true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disclaimer, err := dg.GetSimpleDisclaimer(tt.blueprintType)
			
			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			if disclaimer == "" {
				t.Error("Expected non-empty disclaimer")
			}
		})
	}
}

func TestGetExcludedCosts(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	excluded, err := dg.GetExcludedCosts(string(BlueprintWebApp))
	if err != nil {
		t.Fatalf("Failed to get excluded costs: %v", err)
	}
	
	if len(excluded) == 0 {
		t.Error("Expected non-empty excluded costs list")
	}
	
	// Check that common exclusions are present
	excludedText := strings.Join(excluded, " ")
	if !strings.Contains(excludedText, "Support") {
		t.Error("Expected AWS Support to be in excluded costs")
	}
}

func TestGetIncludedCosts(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	included, err := dg.GetIncludedCosts(string(BlueprintStaticWebsite))
	if err != nil {
		t.Fatalf("Failed to get included costs: %v", err)
	}
	
	if len(included) == 0 {
		t.Error("Expected non-empty included costs list")
	}
	
	// Check that S3 is included for static website
	includedText := strings.Join(included, " ")
	if !strings.Contains(includedText, "S3") {
		t.Error("Expected S3 to be in included costs for static website")
	}
}

func TestFormatDisclaimerForDisplay(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	data, err := dg.GenerateDisclaimer(string(BlueprintWebApp), "us-east-1")
	if err != nil {
		t.Fatalf("Failed to generate disclaimer: %v", err)
	}
	
	formatted := dg.FormatDisclaimerForDisplay(data)
	
	if formatted == "" {
		t.Error("Expected non-empty formatted output")
	}
	
	// Check that formatted output contains key sections
	if !strings.Contains(formatted, "INCLUDED IN ESTIMATE") {
		t.Error("Expected 'INCLUDED IN ESTIMATE' section")
	}
	if !strings.Contains(formatted, "EXCLUDED FROM ESTIMATE") {
		t.Error("Expected 'EXCLUDED FROM ESTIMATE' section")
	}
	if !strings.Contains(formatted, "USAGE ASSUMPTIONS") {
		t.Error("Expected 'USAGE ASSUMPTIONS' section")
	}
	if !strings.Contains(formatted, "ACCURACY") {
		t.Error("Expected 'ACCURACY' section")
	}
	
	// Check for visual markers
	if !strings.Contains(formatted, "✓") {
		t.Error("Expected checkmark for included items")
	}
	if !strings.Contains(formatted, "✗") {
		t.Error("Expected X mark for excluded items")
	}
}

func TestValidateDisclaimerCompleteness(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	tests := []struct {
		name      string
		data      *DisclaimerData
		wantError bool
		errorMsg  string
	}{
		{
			name: "Complete disclaimer",
			data: &DisclaimerData{
				Disclaimer:        "Test disclaimer",
				Included:          []string{"Service 1"},
				Excluded:          []string{"Service 2"},
				Assumptions:       map[string]string{"key": "value"},
				AccuracyStatement: "Test accuracy",
			},
			wantError: false,
		},
		{
			name: "Missing disclaimer text",
			data: &DisclaimerData{
				Disclaimer:        "",
				Included:          []string{"Service 1"},
				Excluded:          []string{"Service 2"},
				Assumptions:       map[string]string{"key": "value"},
				AccuracyStatement: "Test accuracy",
			},
			wantError: true,
			errorMsg:  "disclaimer text is required",
		},
		{
			name: "Empty included list",
			data: &DisclaimerData{
				Disclaimer:        "Test disclaimer",
				Included:          []string{},
				Excluded:          []string{"Service 2"},
				Assumptions:       map[string]string{"key": "value"},
				AccuracyStatement: "Test accuracy",
			},
			wantError: true,
			errorMsg:  "included costs list cannot be empty",
		},
		{
			name: "Empty excluded list",
			data: &DisclaimerData{
				Disclaimer:        "Test disclaimer",
				Included:          []string{"Service 1"},
				Excluded:          []string{},
				Assumptions:       map[string]string{"key": "value"},
				AccuracyStatement: "Test accuracy",
			},
			wantError: true,
			errorMsg:  "excluded costs list cannot be empty",
		},
		{
			name: "Empty assumptions",
			data: &DisclaimerData{
				Disclaimer:        "Test disclaimer",
				Included:          []string{"Service 1"},
				Excluded:          []string{"Service 2"},
				Assumptions:       map[string]string{},
				AccuracyStatement: "Test accuracy",
			},
			wantError: true,
			errorMsg:  "assumptions cannot be empty",
		},
		{
			name: "Missing accuracy statement",
			data: &DisclaimerData{
				Disclaimer:        "Test disclaimer",
				Included:          []string{"Service 1"},
				Excluded:          []string{"Service 2"},
				Assumptions:       map[string]string{"key": "value"},
				AccuracyStatement: "",
			},
			wantError: true,
			errorMsg:  "accuracy statement is required",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := dg.ValidateDisclaimerCompleteness(tt.data)
			
			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestDisclaimerData_AllBlueprintsHaveRequiredFields(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	blueprints := []string{
		string(BlueprintStaticWebsite),
		string(BlueprintWebApp),
		string(BlueprintFullStack),
		string(BlueprintMicroservices),
	}
	
	for _, blueprint := range blueprints {
		t.Run(blueprint, func(t *testing.T) {
			data, err := dg.GenerateDisclaimer(blueprint, "us-east-1")
			if err != nil {
				t.Fatalf("Failed to generate disclaimer: %v", err)
			}
			
			// Validate all required fields are present
			if err := dg.ValidateDisclaimerCompleteness(data); err != nil {
				t.Errorf("Blueprint %s has incomplete disclaimer: %v", blueprint, err)
			}
			
			// Ensure minimum number of items in each list
			if len(data.Included) < 3 {
				t.Errorf("Blueprint %s should have at least 3 included items, got %d", 
					blueprint, len(data.Included))
			}
			
			if len(data.Excluded) < 5 {
				t.Errorf("Blueprint %s should have at least 5 excluded items, got %d", 
					blueprint, len(data.Excluded))
			}
			
			if len(data.Assumptions) < 3 {
				t.Errorf("Blueprint %s should have at least 3 assumptions, got %d", 
					blueprint, len(data.Assumptions))
			}
		})
	}
}

func TestDisclaimerData_RegionSpecific(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	regions := []string{"us-east-1", "eu-west-1", "ap-southeast-1"}
	
	for _, region := range regions {
		t.Run(region, func(t *testing.T) {
			data, err := dg.GenerateDisclaimer(string(BlueprintWebApp), region)
			if err != nil {
				t.Fatalf("Failed to generate disclaimer for region %s: %v", region, err)
			}
			
			if !strings.Contains(data.Disclaimer, region) {
				t.Errorf("Expected disclaimer to mention region %s", region)
			}
		})
	}
}

func TestDisclaimerData_ConsistentExclusions(t *testing.T) {
	dg := NewDisclaimerGenerator()
	
	// All blueprints should exclude AWS Support costs
	blueprints := []string{
		string(BlueprintStaticWebsite),
		string(BlueprintWebApp),
		string(BlueprintFullStack),
		string(BlueprintMicroservices),
	}
	
	for _, blueprint := range blueprints {
		t.Run(blueprint, func(t *testing.T) {
			data, err := dg.GenerateDisclaimer(blueprint, "us-east-1")
			if err != nil {
				t.Fatalf("Failed to generate disclaimer: %v", err)
			}
			
			excludedText := strings.Join(data.Excluded, " ")
			if !strings.Contains(excludedText, "Support") {
				t.Errorf("Blueprint %s should exclude AWS Support costs", blueprint)
			}
		})
	}
}
