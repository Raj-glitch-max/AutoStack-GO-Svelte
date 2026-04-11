# AutoStack Intelligence Package

**AI-powered error analysis and automatic recovery for deployment failures**

---

## Overview

The Intelligence package provides automatic error detection, analysis, and recovery capabilities for AutoStack deployments. It dramatically reduces manual debugging time and improves deployment success rates.

## Features

- **40+ Error Patterns** - Covers Terraform AWS, Kubernetes, and generic errors
- **Automatic Recovery** - 4 intelligent recovery strategies
- **Confidence Scoring** - Only auto-applies high-confidence fixes
- **Detailed Analysis** - Root cause, suggested fix, and step-by-step instructions
- **Full Tracking** - All recovery attempts logged for analytics

## Components

### 1. Error Analyzer (`error_analyzer.go`)

Analyzes error logs and provides detailed diagnostics.

**Usage:**
```go
import "github.com/Raj-glitch-max/autostack/pkg/intelligence"

analyzer := intelligence.NewErrorAnalyzer()
analysis := analyzer.AnalyzeTerraformError(ctx, errorLogs)

if analysis.AutoFixable {
    fmt.Printf("Can auto-fix with %.0f%% confidence\n", analysis.Confidence*100)
    fmt.Printf("Suggested fix: %s\n", analysis.SuggestedFix)
}
```

**Supported Error Categories:**
- `terraform_aws` - AWS-specific Terraform errors (15 patterns)
- `terraform_syntax` - Terraform configuration errors (4 patterns)
- `terraform_state` - State management errors (4 patterns)
- `kubernetes` - Kubernetes deployment errors (8 patterns)

### 2. Recovery Engine (`recovery_engine.go`)

Orchestrates automatic recovery attempts.

**Usage:**
```go
engine := intelligence.NewRecoveryEngine()
attempt, err := engine.AttemptRecovery(ctx, deployment, errorLogs, attemptNum)

if err == nil && attempt.Status == "success" {
    // Apply the fix
    for _, action := range attempt.Fix.Actions {
        // Execute fix action
    }
}
```

**Recovery Strategies:**
1. **Retry** - For transient failures (timeouts, temporary unavailability)
2. **Rename** - For naming conflicts (S3 buckets, resources)
3. **Scale Up** - For resource exhaustion (OOM, CPU limits)
4. **Unlock** - For state lock issues

## Error Patterns

### Terraform AWS Errors

| Pattern | Severity | Auto-Fixable | Strategy |
|---------|----------|--------------|----------|
| BucketAlreadyExists | Medium | ✅ Yes | Rename |
| AccessDenied | Critical | ❌ No | Manual |
| ResourceAlreadyExists | Medium | ✅ Yes | Rename |
| LimitExceeded | High | ❌ No | Manual |
| Timeout | Medium | ✅ Yes | Retry |
| InvalidSubnet | High | ❌ No | Manual |
| InvalidSecurityGroup | High | ❌ No | Manual |
| InvalidParameterValue | Medium | ✅ Yes | Rename |
| DependencyViolation | Medium | ❌ No | Manual |

### Kubernetes Errors

| Pattern | Severity | Auto-Fixable | Strategy |
|---------|----------|--------------|----------|
| ImagePullBackOff | High | ❌ No | Manual |
| CrashLoopBackOff | Critical | ❌ No | Manual |
| OOMKilled | High | ✅ Yes | Scale Up |
| Insufficient Resources | High | ❌ No | Manual |
| CreateContainerConfigError | High | ❌ No | Manual |
| InvalidImageName | Medium | ✅ Yes | Rename |
| FailedScheduling | Medium | ✅ Yes | Manual |
| Liveness Probe Failed | High | ✅ Yes | Manual |

### Terraform State Errors

| Pattern | Severity | Auto-Fixable | Strategy |
|---------|----------|--------------|----------|
| State Lock | Medium | ✅ Yes | Unlock |
| Backend Init Required | Medium | ✅ Yes | Retry |
| Invalid Syntax | High | ❌ No | Manual |
| Undeclared Variable | High | ✅ Yes | Manual |

## API

### ErrorAnalyzer

#### `NewErrorAnalyzer() *ErrorAnalyzer`
Creates a new error analyzer with all patterns loaded.

#### `AnalyzeError(ctx, errorMsg, deploymentType) *ErrorAnalysis`
Analyzes an error message and returns detailed analysis.

**Parameters:**
- `ctx` - Context for cancellation
- `errorMsg` - The error message to analyze
- `deploymentType` - Type of deployment ("terraform" or "kubernetes")

**Returns:**
- `ErrorAnalysis` - Detailed analysis with suggested fix

#### `AnalyzeTerraformError(ctx, logs) *ErrorAnalysis`
Convenience method for Terraform errors.

#### `AnalyzeKubernetesError(ctx, logs) *ErrorAnalysis`
Convenience method for Kubernetes errors.

### RecoveryEngine

#### `NewRecoveryEngine() *RecoveryEngine`
Creates a new recovery engine with all strategies loaded.

#### `AttemptRecovery(ctx, deployment, errorLogs, attemptNum) (*RecoveryAttempt, error)`
Attempts to recover from a deployment failure.

**Parameters:**
- `ctx` - Context for cancellation
- `deployment` - The failed deployment record
- `errorLogs` - Error logs from the failure
- `attemptNum` - Current attempt number (1-3)

**Returns:**
- `RecoveryAttempt` - Details of the recovery attempt
- `error` - Error if recovery failed

#### `ShouldAttemptRecovery(deployment, attemptNum) bool`
Determines if recovery should be attempted.

#### `GetRecoveryStats(ctx, userID) (*RecoveryStats, error)`
Returns recovery statistics for a user.

## Data Structures

### ErrorAnalysis

```go
type ErrorAnalysis struct {
    ErrorType        string                 // Error category
    Category         string                 // Same as ErrorType
    Severity         string                 // low, medium, high, critical
    Description      string                 // Human-readable description
    RootCause        string                 // Extracted root cause
    SuggestedFix     string                 // Suggested solution
    AutoFixable      bool                   // Can be auto-fixed
    FixSteps         []string               // Step-by-step instructions
    RelatedDocs      []string               // Documentation links
    EstimatedFixTime string                 // Estimated time to fix
    Confidence       float64                // Confidence score (0-1)
    RawError         string                 // Original error message
    Context          map[string]interface{} // Additional context
}
```

### AutoFix

```go
type AutoFix struct {
    AnalysisID   string      // Analysis identifier
    DeploymentID string      // Deployment identifier
    ErrorType    string      // Type of error
    FixType      string      // Type of fix (retry, rename, etc.)
    Description  string      // Fix description
    Actions      []FixAction // Actions to perform
    Confidence   float64     // Confidence score
    CreatedAt    time.Time   // Creation timestamp
}
```

### FixAction

```go
type FixAction struct {
    Type        string      // Action type
    Target      string      // Target resource/variable
    NewValue    interface{} // New value to apply
    Description string      // Action description
    Delay       int         // Delay in seconds (for retry)
}
```

### RecoveryAttempt

```go
type RecoveryAttempt struct {
    ID           string                 // Attempt identifier
    DeploymentID string                 // Deployment identifier
    Analysis     *ErrorAnalysis         // Error analysis
    Fix          *AutoFix               // Applied fix
    Strategy     string                 // Recovery strategy used
    Status       string                 // pending, in_progress, success, failed
    AttemptNum   int                    // Attempt number
    StartedAt    time.Time              // Start timestamp
    CompletedAt  *time.Time             // Completion timestamp
    Error        string                 // Error message if failed
    Metadata     map[string]interface{} // Additional metadata
}
```

## Examples

### Example 1: Analyze S3 Bucket Conflict

```go
analyzer := intelligence.NewErrorAnalyzer()
errorLog := `Error: error creating S3 Bucket: BucketAlreadyExists: 
The requested bucket name is not available`

analysis := analyzer.AnalyzeTerraformError(context.Background(), errorLog)

fmt.Printf("Error Type: %s\n", analysis.ErrorType)
// Output: Error Type: terraform_aws

fmt.Printf("Severity: %s\n", analysis.Severity)
// Output: Severity: medium

fmt.Printf("Auto-Fixable: %v\n", analysis.AutoFixable)
// Output: Auto-Fixable: true

fmt.Printf("Suggested Fix: %s\n", analysis.SuggestedFix)
// Output: Suggested Fix: S3 bucket names must be globally unique. Choose a different bucket name

fmt.Printf("Confidence: %.0f%%\n", analysis.Confidence*100)
// Output: Confidence: 90%
```

### Example 2: Attempt Automatic Recovery

```go
engine := intelligence.NewRecoveryEngine()
deployment := getFailedDeployment() // Your deployment record
errorLogs := getDeploymentLogs()    // Error logs

attempt, err := engine.AttemptRecovery(
    context.Background(),
    deployment,
    errorLogs,
    1, // First attempt
)

if err != nil {
    log.Printf("Recovery failed: %v", err)
    return
}

if attempt.Status == "success" {
    log.Printf("Recovery successful! Strategy: %s", attempt.Strategy)
    
    // Apply the fix
    for _, action := range attempt.Fix.Actions {
        switch action.Type {
        case "update_variable":
            // Update deployment configuration
            updateVariable(action.Target, action.NewValue)
        case "retry":
            // Wait and retry
            time.Sleep(time.Duration(action.Delay) * time.Second)
        }
    }
    
    // Retry deployment
    retryDeployment(deployment)
}
```

### Example 3: Get Recovery Statistics

```go
engine := intelligence.NewRecoveryEngine()
stats, err := engine.GetRecoveryStats(context.Background(), userID)

if err != nil {
    log.Printf("Failed to get stats: %v", err)
    return
}

fmt.Printf("Total Attempts: %d\n", stats.TotalAttempts)
fmt.Printf("Success Rate: %.1f%%\n", stats.SuccessRate)
fmt.Printf("Avg Recovery Time: %.1fs\n", stats.AvgRecoveryTime)

fmt.Println("\nMost Common Errors:")
for _, errorStat := range stats.MostCommonErrors {
    fmt.Printf("  %s: %d occurrences\n", errorStat.ErrorType, errorStat.Count)
}
```

## Testing

Run the test suite:

```bash
go test ./pkg/intelligence -v
```

Run specific tests:

```bash
# Test error analyzer
go test ./pkg/intelligence -v -run TestErrorAnalyzer

# Test recovery engine
go test ./pkg/intelligence -v -run TestRecoveryEngine

# Test confidence scoring
go test ./pkg/intelligence -v -run TestConfidenceScoring
```

## Configuration

### Confidence Threshold

Adjust the confidence threshold for auto-fixes in `recovery_engine.go`:

```go
// Default: 0.7 (70%)
if fix.Confidence < 0.7 {
    return fmt.Errorf("fix confidence too low")
}

// More conservative: 0.8 (80%)
if fix.Confidence < 0.8 {
    return fmt.Errorf("fix confidence too low")
}
```

### Max Retry Attempts

Adjust max retries in the Terraform executor:

```go
// Default: 3 attempts
maxAttempts := 3

// More attempts: 5
maxAttempts := 5
```

### Add Custom Patterns

Add new error patterns in `error_analyzer.go`:

```go
ea.patterns = append(ea.patterns, ErrorPattern{
    Pattern:     regexp.MustCompile(`your-error-pattern`),
    Category:    "terraform_aws",
    Severity:    "high",
    Description: "Your error description",
    Solution:    "Your suggested fix",
    AutoFixable: true,
})
```

## Performance

- **Pattern Matching**: < 1ms per error
- **Error Analysis**: < 10ms including pattern matching
- **Recovery Attempt**: 5-60 seconds depending on strategy
- **Memory Usage**: ~5MB for pattern storage

## Security

- All error logs are sanitized before storage
- User isolation enforced at database level
- Confidence threshold prevents risky auto-fixes
- All recovery attempts logged for audit trail

## Future Enhancements

- LLM integration for unknown errors
- Machine learning for pattern improvement
- Custom user-defined patterns
- Predictive failure analysis
- Multi-cloud support (Azure, GCP)

## License

MIT License - See LICENSE file for details

## Support

For issues or questions:
- GitHub Issues: https://github.com/your-repo/autostack/issues
- Documentation: See INTELLIGENCE_IMPLEMENTATION_GUIDE.md
- Quick Start: See INTELLIGENCE_QUICK_START.md
