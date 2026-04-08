package intelligence

import (
	"context"
	"testing"
)

func TestErrorAnalyzer_TerraformAWSErrors(t *testing.T) {
	analyzer := NewErrorAnalyzer()
	ctx := context.Background()

	tests := []struct {
		name           string
		errorLog       string
		expectedType   string
		expectedSeverity string
		autoFixable    bool
	}{
		{
			name: "S3 Bucket Already Exists",
			errorLog: `Error: error creating S3 Bucket: BucketAlreadyExists: 
The requested bucket name is not available`,
			expectedType:   "terraform_aws",
			expectedSeverity: "medium",
			autoFixable:    true,
		},
		{
			name: "AWS Access Denied",
			errorLog: `Error: AccessDenied: User is not authorized to perform: 
ecs:CreateCluster on resource`,
			expectedType:   "terraform_aws",
			expectedSeverity: "critical",
			autoFixable:    false,
		},
		{
			name: "Resource Already Exists",
			errorLog: `Error: ResourceAlreadyExists: The resource with name 'my-resource' already exists`,
			expectedType:   "terraform_aws",
			expectedSeverity: "medium",
			autoFixable:    true,
		},
		{
			name: "Timeout Waiting for State",
			errorLog: `Error: timeout while waiting for state to become 'available'`,
			expectedType:   "terraform_aws",
			expectedSeverity: "medium",
			autoFixable:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzer.AnalyzeTerraformError(ctx, tt.errorLog)

			if analysis.ErrorType != tt.expectedType {
				t.Errorf("Expected error type %s, got %s", tt.expectedType, analysis.ErrorType)
			}

			if analysis.Severity != tt.expectedSeverity {
				t.Errorf("Expected severity %s, got %s", tt.expectedSeverity, analysis.Severity)
			}

			if analysis.AutoFixable != tt.autoFixable {
				t.Errorf("Expected autoFixable %v, got %v", tt.autoFixable, analysis.AutoFixable)
			}

			if analysis.SuggestedFix == "" {
				t.Error("Expected suggested fix to be non-empty")
			}

			if len(analysis.FixSteps) == 0 {
				t.Error("Expected fix steps to be non-empty")
			}
		})
	}
}

func TestErrorAnalyzer_KubernetesErrors(t *testing.T) {
	analyzer := NewErrorAnalyzer()
	ctx := context.Background()

	tests := []struct {
		name           string
		errorLog       string
		expectedType   string
		expectedSeverity string
		autoFixable    bool
	}{
		{
			name:           "Image Pull Back Off",
			errorLog:       `Pod status: ImagePullBackOff - Failed to pull image`,
			expectedType:   "kubernetes",
			expectedSeverity: "high",
			autoFixable:    false,
		},
		{
			name:           "OOM Killed",
			errorLog:       `Container killed: OOMKilled - Memory limit exceeded`,
			expectedType:   "kubernetes",
			expectedSeverity: "high",
			autoFixable:    true,
		},
		{
			name:           "Crash Loop Back Off",
			errorLog:       `Pod status: CrashLoopBackOff - Container is crashing repeatedly`,
			expectedType:   "kubernetes",
			expectedSeverity: "critical",
			autoFixable:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzer.AnalyzeKubernetesError(ctx, tt.errorLog)

			if analysis.ErrorType != tt.expectedType {
				t.Errorf("Expected error type %s, got %s", tt.expectedType, analysis.ErrorType)
			}

			if analysis.Severity != tt.expectedSeverity {
				t.Errorf("Expected severity %s, got %s", tt.expectedSeverity, analysis.Severity)
			}

			if analysis.AutoFixable != tt.autoFixable {
				t.Errorf("Expected autoFixable %v, got %v", tt.autoFixable, analysis.AutoFixable)
			}
		})
	}
}

func TestErrorAnalyzer_UnknownError(t *testing.T) {
	analyzer := NewErrorAnalyzer()
	ctx := context.Background()

	errorLog := "Some completely unknown error that doesn't match any pattern"
	analysis := analyzer.AnalyzeError(ctx, errorLog, "terraform")

	if analysis.ErrorType != "unknown" {
		t.Errorf("Expected error type 'unknown', got %s", analysis.ErrorType)
	}

	if analysis.Confidence >= 0.5 {
		t.Errorf("Expected low confidence for unknown error, got %.2f", analysis.Confidence)
	}

	if analysis.AutoFixable {
		t.Error("Unknown errors should not be auto-fixable")
	}
}

func TestErrorAnalyzer_ConfidenceScoring(t *testing.T) {
	analyzer := NewErrorAnalyzer()
	ctx := context.Background()

	// Known error should have high confidence
	knownError := "Error: BucketAlreadyExists: The requested bucket name is not available"
	knownAnalysis := analyzer.AnalyzeTerraformError(ctx, knownError)

	if knownAnalysis.Confidence < 0.8 {
		t.Errorf("Expected high confidence for known error, got %.2f", knownAnalysis.Confidence)
	}

	// Unknown error should have low confidence
	unknownError := "Some random error message"
	unknownAnalysis := analyzer.AnalyzeTerraformError(ctx, unknownError)

	if unknownAnalysis.Confidence >= 0.5 {
		t.Errorf("Expected low confidence for unknown error, got %.2f", unknownAnalysis.Confidence)
	}
}
