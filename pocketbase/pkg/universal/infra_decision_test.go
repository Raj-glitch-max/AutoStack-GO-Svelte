package universal

import (
	"testing"
)

func TestInfraDecision_StaticSite(t *testing.T) {
	profile := AppProfile{
		ProjectName:       "my-site",
		IsStaticSite:      true,
		AppType:           AppTypeStaticSite,
		ResourceProfile:   ResourceMinimal,
		ExpectsWebTraffic: true,
	}
	spec := DecideInfrastructure(profile, UserPreferences{Region: "us-east-1"})

	if spec.Compute.Type != ComputeTypeS3Static {
		t.Errorf("expected S3Static compute, got %s", spec.Compute.Type)
	}
	if spec.LoadBalancer != nil {
		t.Error("static site should not have a load balancer")
	}
	if spec.CDN == nil {
		t.Error("static site should have CDN")
	}
	if spec.Network.EnableNATGateway {
		t.Error("static site should not need NAT gateway")
	}
}

func TestInfraDecision_WebSocketApp(t *testing.T) {
	profile := AppProfile{
		ProjectName:       "chat-app",
		Language:          "nodejs",
		Framework:         "socketio",
		AppType:           AppTypeWebSocket,
		Port:              3000,
		ResourceProfile:   ResourceStandard,
		ExpectsWebTraffic: true,
		ExpectsWebSocket:  true,
	}
	spec := DecideInfrastructure(profile, UserPreferences{Region: "us-east-1"})

	if spec.LoadBalancer == nil {
		t.Fatal("websocket app needs a load balancer")
	}
	if !spec.LoadBalancer.StickySession {
		t.Error("websocket app needs sticky sessions on ALB")
	}
	if spec.Compute.Type != ComputeTypeECSFargate {
		t.Errorf("expected ECS Fargate, got %s", spec.Compute.Type)
	}
}

func TestInfraDecision_BackgroundWorker(t *testing.T) {
	profile := AppProfile{
		ProjectName:        "worker",
		Language:           "python",
		Framework:          "celery",
		AppType:            AppTypeWorker,
		ResourceProfile:    ResourceStandard,
		IsBackgroundWorker: true,
		ExpectsWebTraffic:  false,
	}
	spec := DecideInfrastructure(profile, UserPreferences{Region: "us-east-1"})

	if spec.LoadBalancer != nil {
		t.Error("background worker should not have a load balancer")
	}
	if spec.Compute.PublicEndpoint {
		t.Error("background worker should not have a public endpoint")
	}
}

func TestInfraDecision_WithPostgresAndRedis(t *testing.T) {
	profile := AppProfile{
		ProjectName:     "full-app",
		Language:        "python",
		Framework:       "django",
		AppType:         AppTypeFullStack,
		Port:            8000,
		ResourceProfile: ResourceStandard,
		RequiredServices: []RequiredService{
			{ServiceType: "postgresql", IsCritical: true, EnvVarName: "DATABASE_URL"},
			{ServiceType: "redis", IsCritical: true, EnvVarName: "REDIS_URL"},
		},
		ExpectsWebTraffic: true,
	}
	spec := DecideInfrastructure(profile, UserPreferences{Region: "us-east-1"})

	if len(spec.ManagedServices) != 2 {
		t.Errorf("expected 2 managed services, got %d", len(spec.ManagedServices))
	}

	found := map[string]bool{}
	for _, svc := range spec.ManagedServices {
		found[svc.Type+":"+svc.Engine] = true
	}
	if !found["rds:postgres"] {
		t.Error("expected RDS postgres")
	}
	if !found["elasticache:redis"] {
		t.Error("expected ElastiCache redis")
	}

	if !spec.Network.EnableNATGateway {
		t.Error("app with managed services needs NAT gateway")
	}
}

func TestInfraDecision_HighAvailability(t *testing.T) {
	profile := AppProfile{
		ProjectName:     "prod-app",
		Language:        "go",
		AppType:         AppTypeAPI,
		Port:            8080,
		ResourceProfile: ResourceStandard,
		RequiredServices: []RequiredService{
			{ServiceType: "postgresql", IsCritical: true},
		},
		ExpectsWebTraffic: true,
	}
	spec := DecideInfrastructure(profile, UserPreferences{
		Region:           "us-east-1",
		HighAvailability: true,
	})

	for _, svc := range spec.ManagedServices {
		if svc.Type == "rds" && !svc.MultiAZ {
			t.Error("high availability should enable Multi-AZ for RDS")
		}
	}
}

func TestCostEstimate_StaticSite(t *testing.T) {
	spec := InfrastructureSpec{
		ProjectName: "my-site",
		Region:      "us-east-1",
		Compute:     ComputeSpec{Type: ComputeTypeS3Static},
		CDN:         &CDNSpec{Type: "cloudfront"},
		Network:     NetworkSpec{},
	}
	estimate := EstimateCost(spec)

	if estimate.MonthlyEstimate <= 0 {
		t.Error("cost estimate must be positive")
	}
	if estimate.MonthlyMin > estimate.MonthlyEstimate {
		t.Error("min must be <= estimate")
	}
	if estimate.MonthlyEstimate > estimate.MonthlyMax {
		t.Error("estimate must be <= max")
	}
}

func TestCostEstimate_FullStack(t *testing.T) {
	spec := InfrastructureSpec{
		ProjectName: "full-app",
		Region:      "us-east-1",
		Compute: ComputeSpec{
			Type:   ComputeTypeECSFargate,
			CPU:    512,
			Memory: 1024,
		},
		LoadBalancer: &LoadBalancerSpec{Type: "alb"},
		ManagedServices: []ManagedServiceSpec{
			{Type: "rds", Engine: "postgres", InstanceClass: "db.t3.micro", StorageGB: 20},
			{Type: "elasticache", Engine: "redis", NodeType: "cache.t3.micro"},
		},
		Network: NetworkSpec{EnableNATGateway: true},
	}
	estimate := EstimateCost(spec)

	if estimate.MonthlyEstimate < 30 {
		t.Errorf("full stack app should cost at least $30/month, got $%.2f", estimate.MonthlyEstimate)
	}
	if len(estimate.Breakdown) == 0 {
		t.Error("breakdown should not be empty")
	}
}
