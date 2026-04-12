package validation

import (
	"testing"
)

func TestValidateBlueprint(t *testing.T) {
	tests := []struct {
		name      string
		blueprint string
		wantErr   bool
	}{
		{
			name:      "valid static-website",
			blueprint: "static-website",
			wantErr:   false,
		},
		{
			name:      "valid web-application",
			blueprint: "web-application",
			wantErr:   false,
		},
		{
			name:      "valid full-stack-app",
			blueprint: "full-stack-app",
			wantErr:   false,
		},
		{
			name:      "valid microservices",
			blueprint: "microservices",
			wantErr:   false,
		},
		{
			name:      "empty blueprint",
			blueprint: "",
			wantErr:   true,
		},
		{
			name:      "unsupported blueprint",
			blueprint: "invalid-blueprint",
			wantErr:   true,
		},
		{
			name:      "case sensitive - uppercase",
			blueprint: "STATIC-WEBSITE",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBlueprint(tt.blueprint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBlueprint() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRegion(t *testing.T) {
	tests := []struct {
		name    string
		region  string
		wantErr bool
	}{
		{
			name:    "valid us-east-1",
			region:  "us-east-1",
			wantErr: false,
		},
		{
			name:    "valid us-west-2",
			region:  "us-west-2",
			wantErr: false,
		},
		{
			name:    "valid eu-west-1",
			region:  "eu-west-1",
			wantErr: false,
		},
		{
			name:    "valid ap-southeast-1",
			region:  "ap-southeast-1",
			wantErr: false,
		},
		{
			name:    "empty region",
			region:  "",
			wantErr: true,
		},
		{
			name:    "invalid format",
			region:  "invalid-region",
			wantErr: true,
		},
		{
			name:    "unknown region",
			region:  "us-fake-1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRegion(tt.region)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRegion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCostEstimateRequest(t *testing.T) {
	tests := []struct {
		name      string
		blueprint string
		region    string
		variables map[string]interface{}
		wantErr   bool
	}{
		{
			name:      "valid static-website request",
			blueprint: "static-website",
			region:    "us-east-1",
			variables: map[string]interface{}{
				"storage_gb":         10.0,
				"requests_per_month": 10000,
			},
			wantErr: false,
		},
		{
			name:      "valid web-application request",
			blueprint: "web-application",
			region:    "us-west-2",
			variables: map[string]interface{}{
				"vcpu":       0.25,
				"memory":     0.5,
				"task_count": 1,
			},
			wantErr: false,
		},
		{
			name:      "invalid blueprint",
			blueprint: "invalid",
			region:    "us-east-1",
			variables: nil,
			wantErr:   true,
		},
		{
			name:      "invalid region",
			blueprint: "static-website",
			region:    "invalid-region",
			variables: nil,
			wantErr:   true,
		},
		{
			name:      "invalid configuration",
			blueprint: "web-application",
			region:    "us-east-1",
			variables: map[string]interface{}{
				"vcpu": 99.0, // Invalid vCPU value
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCostEstimateRequest(tt.blueprint, tt.region, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCostEstimateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateStaticWebsiteConfig(t *testing.T) {
	tests := []struct {
		name      string
		variables map[string]interface{}
		wantErr   bool
	}{
		{
			name: "valid configuration",
			variables: map[string]interface{}{
				"storage_gb":         10.0,
				"requests_per_month": 10000,
				"data_transfer_gb":   100.0,
				"has_custom_domain":  true,
			},
			wantErr: false,
		},
		{
			name:      "nil variables",
			variables: nil,
			wantErr:   false,
		},
		{
			name: "negative storage",
			variables: map[string]interface{}{
				"storage_gb": -10.0,
			},
			wantErr: true,
		},
		{
			name: "negative requests",
			variables: map[string]interface{}{
				"requests_per_month": -1000,
			},
			wantErr: true,
		},
		{
			name: "invalid custom domain type",
			variables: map[string]interface{}{
				"has_custom_domain": "yes", // Should be boolean
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStaticWebsiteConfig(tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateStaticWebsiteConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWebApplicationConfig(t *testing.T) {
	tests := []struct {
		name      string
		variables map[string]interface{}
		wantErr   bool
	}{
		{
			name: "valid configuration",
			variables: map[string]interface{}{
				"vcpu":                0.25,
				"memory":              0.5,
				"task_count":          1,
				"db_instance_class":   "db.t3.micro",
				"db_storage_gb":       20,
				"db_engine":           "postgres",
				"lcu_hours":           730.0,
				"nat_data_gb":         50.0,
				"cloudwatch_logs_gb":  5.0,
			},
			wantErr: false,
		},
		{
			name: "invalid vcpu",
			variables: map[string]interface{}{
				"vcpu": 3.0, // Not a valid Fargate vCPU value
			},
			wantErr: true,
		},
		{
			name: "invalid task count",
			variables: map[string]interface{}{
				"task_count": 0, // Must be at least 1
			},
			wantErr: true,
		},
		{
			name: "invalid db instance class",
			variables: map[string]interface{}{
				"db_instance_class": "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid db engine",
			variables: map[string]interface{}{
				"db_engine": "mongodb", // Not a valid RDS engine
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebApplicationConfig(tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWebApplicationConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFullStackConfig(t *testing.T) {
	tests := []struct {
		name      string
		variables map[string]interface{}
		wantErr   bool
	}{
		{
			name: "valid configuration",
			variables: map[string]interface{}{
				"frontend_storage_gb":     5.0,
				"frontend_requests":       50000,
				"frontend_data_transfer":  200.0,
				"backend_vcpu":            0.5,
				"backend_memory":          1.0,
				"backend_task_count":      2,
				"db_instance_class":       "db.t3.small",
				"db_storage_gb":           50,
				"db_engine":               "postgres",
				"lcu_hours":               1460.0,
				"nat_data_gb":             100.0,
				"cloudwatch_logs_gb":      10.0,
				"has_custom_domain":       true,
			},
			wantErr: false,
		},
		{
			name: "invalid frontend storage",
			variables: map[string]interface{}{
				"frontend_storage_gb": -5.0,
			},
			wantErr: true,
		},
		{
			name: "invalid backend vcpu",
			variables: map[string]interface{}{
				"backend_vcpu": 7.0, // Not a valid Fargate vCPU
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFullStackConfig(tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFullStackConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMicroservicesConfig(t *testing.T) {
	tests := []struct {
		name      string
		variables map[string]interface{}
		wantErr   bool
	}{
		{
			name: "valid configuration",
			variables: map[string]interface{}{
				"service_count":       3,
				"service_vcpu":        0.5,
				"service_memory":      1.0,
				"service_tasks":       2,
				"db_instance_class":   "db.t3.medium",
				"db_storage_gb":       100,
				"db_engine":           "postgres",
				"storage_gb":          50.0,
				"requests_per_month":  100000,
				"data_transfer_gb":    300.0,
				"lcu_hours":           2190.0,
				"nat_data_gb":         200.0,
				"cloudwatch_logs_gb":  20.0,
				"api_gateway_reqs":    1000000,
				"sqs_requests":        1000000,
				"elasticache_nodes":   1,
			},
			wantErr: false,
		},
		{
			name: "invalid service count",
			variables: map[string]interface{}{
				"service_count": 0, // Must be at least 1
			},
			wantErr: true,
		},
		{
			name: "invalid elasticache nodes",
			variables: map[string]interface{}{
				"elasticache_nodes": -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMicroservicesConfig(tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMicroservicesConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePositiveNumber(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		fieldName string
		wantErr   bool
	}{
		{
			name:      "valid float64",
			value:     10.5,
			fieldName: "test_field",
			wantErr:   false,
		},
		{
			name:      "valid int",
			value:     10,
			fieldName: "test_field",
			wantErr:   false,
		},
		{
			name:      "zero is valid",
			value:     0,
			fieldName: "test_field",
			wantErr:   false,
		},
		{
			name:      "negative number",
			value:     -10.5,
			fieldName: "test_field",
			wantErr:   true,
		},
		{
			name:      "invalid type",
			value:     "not a number",
			fieldName: "test_field",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePositiveNumber(tt.value, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePositiveNumber() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePositiveInteger(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		fieldName string
		wantErr   bool
	}{
		{
			name:      "valid int",
			value:     10,
			fieldName: "test_field",
			wantErr:   false,
		},
		{
			name:      "valid float64 whole number",
			value:     10.0,
			fieldName: "test_field",
			wantErr:   false,
		},
		{
			name:      "zero is invalid",
			value:     0,
			fieldName: "test_field",
			wantErr:   true,
		},
		{
			name:      "negative number",
			value:     -10,
			fieldName: "test_field",
			wantErr:   true,
		},
		{
			name:      "float with decimal",
			value:     10.5,
			fieldName: "test_field",
			wantErr:   true,
		},
		{
			name:      "invalid type",
			value:     "not a number",
			fieldName: "test_field",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePositiveInteger(tt.value, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePositiveInteger() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFargateVCPU(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "valid 0.25",
			value:   0.25,
			wantErr: false,
		},
		{
			name:    "valid 0.5",
			value:   0.5,
			wantErr: false,
		},
		{
			name:    "valid 1",
			value:   1.0,
			wantErr: false,
		},
		{
			name:    "valid 2",
			value:   2.0,
			wantErr: false,
		},
		{
			name:    "valid 4",
			value:   4.0,
			wantErr: false,
		},
		{
			name:    "valid 8",
			value:   8.0,
			wantErr: false,
		},
		{
			name:    "valid 16",
			value:   16.0,
			wantErr: false,
		},
		{
			name:    "invalid 3",
			value:   3.0,
			wantErr: true,
		},
		{
			name:    "invalid 0.75",
			value:   0.75,
			wantErr: true,
		},
		{
			name:    "invalid type",
			value:   "not a number",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFargateVCPU(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFargateVCPU() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRDSInstanceClass(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "valid db.t3.micro",
			value:   "db.t3.micro",
			wantErr: false,
		},
		{
			name:    "valid db.m5.large",
			value:   "db.m5.large",
			wantErr: false,
		},
		{
			name:    "valid db.r5.xlarge",
			value:   "db.r5.xlarge",
			wantErr: false,
		},
		{
			name:    "missing db prefix",
			value:   "t3.micro",
			wantErr: true,
		},
		{
			name:    "invalid format",
			value:   "db.invalid",
			wantErr: true,
		},
		{
			name:    "unknown family",
			value:   "db.unknown.micro",
			wantErr: true,
		},
		{
			name:    "unknown size",
			value:   "db.t3.unknown",
			wantErr: true,
		},
		{
			name:    "empty string",
			value:   "",
			wantErr: true,
		},
		{
			name:    "invalid type",
			value:   123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRDSInstanceClass(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRDSInstanceClass() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRDSEngine(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "valid postgres",
			value:   "postgres",
			wantErr: false,
		},
		{
			name:    "valid mysql",
			value:   "mysql",
			wantErr: false,
		},
		{
			name:    "valid mariadb",
			value:   "mariadb",
			wantErr: false,
		},
		{
			name:    "valid aurora-postgresql",
			value:   "aurora-postgresql",
			wantErr: false,
		},
		{
			name:    "invalid engine",
			value:   "mongodb",
			wantErr: true,
		},
		{
			name:    "empty string",
			value:   "",
			wantErr: true,
		},
		{
			name:    "invalid type",
			value:   123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRDSEngine(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRDSEngine() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBoolean(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		fieldName string
		wantErr   bool
	}{
		{
			name:      "valid true",
			value:     true,
			fieldName: "test_field",
			wantErr:   false,
		},
		{
			name:      "valid false",
			value:     false,
			fieldName: "test_field",
			wantErr:   false,
		},
		{
			name:      "invalid string",
			value:     "true",
			fieldName: "test_field",
			wantErr:   true,
		},
		{
			name:      "invalid int",
			value:     1,
			fieldName: "test_field",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBoolean(tt.value, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBoolean() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
