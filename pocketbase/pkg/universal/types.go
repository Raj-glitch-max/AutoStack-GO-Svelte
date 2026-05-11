package universal

import "time"

// ─── Input types ─────────────────────────────────────────────────────────────

// RawAppDescription is produced by Stage 1 (InputNormalizer).
// All subsequent stages are input-type-agnostic — they only see this struct.
type RawAppDescription struct {
	InputType       string // "git", "image", "compose", "source", "natural_language"
	SourceDir       string // path to cloned/extracted source (if applicable)
	Files           []string
	DockerImage     string
	DockerfilePath  string
	ComposeServices []ComposeService
	NaturalLanguage string
	EnvVarKeys      []string
	ProjectName     string
}

// ComposeService represents one service from a docker-compose.yml
type ComposeService struct {
	Name               string
	Image              string
	BuildPath          string
	Ports              []string
	Environment        map[string]string
	DependsOn          []string
	Volumes            []string
	IsDatabase         bool
	IsManagedByAWS     bool   // true = use RDS/ElastiCache, not ECS
	ManagedServiceType string // "rds_postgres", "elasticache_redis", etc.
}

// ─── App profile ──────────────────────────────────────────────────────────────

// AppProfile is the complete, structured understanding of what the app IS.
// Produced by Stage 2 (AppAnalyzer). Every field is deterministic.
type AppProfile struct {
	ProjectName     string
	Language        string      // "go", "nodejs", "python", "java", "ruby", "php", "rust", "dotnet", "static"
	Framework       string      // "express", "django", "gin", etc.
	AppType         AppType
	Port            int
	HasDockerfile   bool
	DockerfilePath  string
	ResourceProfile ResourceProfile
	RequiredServices []RequiredService
	EnvVarKeys      []string
	NeedsFileStorage   bool
	NeedsObjectStorage bool
	ExpectsWebTraffic  bool
	ExpectsWebSocket   bool
	IsBackgroundWorker bool
	IsScheduledJob     bool
	IsStaticSite       bool
	DetectionConfidence float64
	AmbiguousFields     []string
}

// AppType classifies the application's runtime behaviour
type AppType string

const (
	AppTypeAPI         AppType = "api"
	AppTypeFullStack   AppType = "fullstack"
	AppTypeStaticSite  AppType = "static_site"
	AppTypeWorker      AppType = "worker"
	AppTypeWebSocket   AppType = "websocket"
	AppTypeMLInference AppType = "ml_inference"
	AppTypeScheduled   AppType = "scheduled_job"
	AppTypeCLI         AppType = "cli"
)

// ResourceProfile maps to ECS Fargate CPU/memory combinations
type ResourceProfile string

const (
	ResourceMinimal  ResourceProfile = "minimal"       // 0.25 vCPU, 512 MB
	ResourceStandard ResourceProfile = "standard"      // 0.5 vCPU, 1 GB
	ResourceCompute  ResourceProfile = "compute_heavy" // 2 vCPU, 4 GB
	ResourceMemory   ResourceProfile = "memory_heavy"  // 1 vCPU, 8 GB
	ResourceGPU      ResourceProfile = "gpu"           // GPU instance
)

// RequiredService is a managed AWS service the app depends on
type RequiredService struct {
	ServiceType string // "postgresql", "mysql", "redis", "mongodb", "rabbitmq", "kafka", "opensearch"
	Version     string
	IsCritical  bool
	EnvVarName  string
}

// ─── Infrastructure spec ──────────────────────────────────────────────────────

// InfrastructureSpec is the complete, validated description of what to build.
// Produced by Stage 3 (InfraDecisionEngine). Input to Stage 5 (TerraformGenerator).
type InfrastructureSpec struct {
	ProjectName     string
	Region          string
	AccountID       string
	Compute         ComputeSpec
	LoadBalancer    *LoadBalancerSpec
	CDN             *CDNSpec
	ManagedServices []ManagedServiceSpec
	Storage         []StorageSpec
	Network         NetworkSpec
	SSL             *SSLSpec
	EstimatedMonthlyCostUSD float64
	GeneratedAt             time.Time
	SpecVersion             string
}

// ComputeSpec describes the compute layer
type ComputeSpec struct {
	Type              ComputeType
	CPU               int    // ECS CPU units (256, 512, 1024, 2048, 4096)
	Memory            int    // MB
	Port              int
	PublicEndpoint    bool
	MinTasks          int
	MaxTasks          int
	EnableAutoScaling bool
	ScalingMetric     string // "cpu", "memory", "requests"
	ScalingThreshold  int    // percentage
	InstanceType      string // for EC2-backed ECS
}

// ComputeType enumerates supported compute backends
type ComputeType string

const (
	ComputeTypeECSFargate   ComputeType = "ecs_fargate"
	ComputeTypeECSEC2       ComputeType = "ecs_ec2"
	ComputeTypeECSScheduled ComputeType = "ecs_scheduled"
	ComputeTypeS3Static     ComputeType = "s3_static"
	ComputeTypeLambda       ComputeType = "lambda"
)

// LoadBalancerSpec describes the load balancer
type LoadBalancerSpec struct {
	Type          string // "alb", "nlb"
	EnableHTTPS   bool
	StickySession bool
	CustomDomain  string
}

// CDNSpec describes the CDN layer
type CDNSpec struct {
	Type         string // "cloudfront"
	CustomDomain string
}

// ManagedServiceSpec describes a managed AWS service
type ManagedServiceSpec struct {
	Type          string // "rds", "elasticache", "documentdb", "amazon_mq", "msk", "opensearch"
	Engine        string // "postgres", "mysql", "redis", "rabbitmq"
	EngineVersion string
	InstanceClass string
	NodeType      string // for ElastiCache
	InstanceType  string // for MSK
	BrokerNodes   int    // for MSK
	MultiAZ       bool
	StorageGB     int
	EnvVarName    string // env var the app uses for the connection string
}

// StorageSpec describes persistent storage
type StorageSpec struct {
	Type       string // "s3", "efs"
	BucketName string
	MountPath  string // for EFS
	EnvVarName string
}

// NetworkSpec describes the VPC and subnets
type NetworkSpec struct {
	CreateVPC        bool
	VPCCidr          string
	PublicSubnets    int
	PrivateSubnets   int
	EnableNATGateway bool
}

// SSLSpec describes SSL/TLS configuration
type SSLSpec struct {
	Provider     string // "acm"
	AutoValidate bool
	CustomDomain string
}

// ─── User preferences ─────────────────────────────────────────────────────────

// UserPreferences are the optional overrides the user can provide
type UserPreferences struct {
	Region           string
	HighAvailability bool
	EnableCDN        bool
	CustomDomain     string
	Environment      string // "production", "staging"
	MaxTasks         int
}

// ─── Deployment outputs ───────────────────────────────────────────────────────

// DeploymentOutputs are parsed from `terraform output -json` after apply
type DeploymentOutputs struct {
	AppEndpoint  string
	ResourceARNs map[string]string
	EnvVars      map[string]string
	RawOutputs   map[string]interface{}
}

// ─── Cost estimate ────────────────────────────────────────────────────────────

// UniversalCostEstimate is the cost estimate shown before deployment
type UniversalCostEstimate struct {
	MonthlyMin      float64
	MonthlyEstimate float64
	MonthlyMax      float64
	Currency        string
	Breakdown       map[string]float64
}

// ─── API request/response ─────────────────────────────────────────────────────

// UniversalDeployRequest is the body for POST /api/deployments/universal
type UniversalDeployRequest struct {
	ProjectID       string `json:"project_id"`
	InputType       string `json:"input_type"` // "git","image","compose","source","natural_language"
	GitURL          string `json:"git_url,omitempty"`
	DockerImage     string `json:"docker_image,omitempty"`
	ComposeContent  string `json:"compose_content,omitempty"`
	NaturalLanguage string `json:"natural_language,omitempty"`
	Region          string `json:"region,omitempty"`
	HighAvailability bool  `json:"high_availability,omitempty"`
	EnableCDN       bool   `json:"enable_cdn,omitempty"`
	CustomDomain    string `json:"custom_domain,omitempty"`
	Environment     string `json:"environment,omitempty"`
	AWSCredentialID string `json:"aws_credential_id"`
}

// UniversalDeployResponse is the immediate response (analysis phase)
type UniversalDeployResponse struct {
	DeploymentID        string                `json:"deployment_id"`
	Status              string                `json:"status"`
	AppProfile          AppProfile            `json:"app_profile"`
	InfrastructureSpec  InfrastructureSpec    `json:"infrastructure_spec"`
	CostEstimate        UniversalCostEstimate `json:"cost_estimate"`
	RequiresConfirmation bool                 `json:"requires_confirmation"`
}
