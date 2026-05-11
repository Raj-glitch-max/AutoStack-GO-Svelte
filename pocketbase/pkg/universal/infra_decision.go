package universal

import (
	"fmt"
	"time"
)

// DecideInfrastructure converts an AppProfile into an InfrastructureSpec.
// This is pure Go logic — no AI, no randomness. Every decision is a rule.
func DecideInfrastructure(profile AppProfile, prefs UserPreferences) InfrastructureSpec {
	spec := InfrastructureSpec{
		ProjectName: sanitizeName(profile.ProjectName),
		Region:      coalesce(prefs.Region, "us-east-1"),
		GeneratedAt: time.Now(),
		SpecVersion: "1.0",
	}

	// ── Compute decision ──────────────────────────────────────────────
	switch {
	case profile.IsStaticSite:
		spec.Compute = ComputeSpec{Type: ComputeTypeS3Static}

	case profile.IsScheduledJob:
		spec.Compute = ComputeSpec{
			Type:   ComputeTypeECSScheduled,
			CPU:    resourceToCPU(profile.ResourceProfile),
			Memory: resourceToMemory(profile.ResourceProfile),
		}

	case profile.IsBackgroundWorker:
		spec.Compute = ComputeSpec{
			Type:           ComputeTypeECSFargate,
			CPU:            resourceToCPU(profile.ResourceProfile),
			Memory:         resourceToMemory(profile.ResourceProfile),
			PublicEndpoint: false,
			MinTasks:       1,
			MaxTasks:       scalingMaxFromPrefs(prefs),
		}

	case profile.ResourceProfile == ResourceGPU:
		spec.Compute = ComputeSpec{
			Type:         ComputeTypeECSEC2,
			InstanceType: "g4dn.xlarge",
			CPU:          4096,
			Memory:       16384,
			Port:         profile.Port,
			PublicEndpoint: true,
		}

	default:
		spec.Compute = ComputeSpec{
			Type:              ComputeTypeECSFargate,
			CPU:               resourceToCPU(profile.ResourceProfile),
			Memory:            resourceToMemory(profile.ResourceProfile),
			Port:              profile.Port,
			PublicEndpoint:    profile.ExpectsWebTraffic,
			MinTasks:          1,
			MaxTasks:          scalingMaxFromPrefs(prefs),
			EnableAutoScaling: true,
			ScalingMetric:     "cpu",
			ScalingThreshold:  70,
		}
	}

	// ── Load balancer decision ────────────────────────────────────────
	if profile.ExpectsWebTraffic && !profile.IsStaticSite {
		spec.LoadBalancer = &LoadBalancerSpec{
			Type:          "alb",
			EnableHTTPS:   true,
			StickySession: profile.ExpectsWebSocket,
			CustomDomain:  prefs.CustomDomain,
		}
	}

	// ── CDN decision ──────────────────────────────────────────────────
	if profile.IsStaticSite || prefs.EnableCDN {
		spec.CDN = &CDNSpec{
			Type:         "cloudfront",
			CustomDomain: prefs.CustomDomain,
		}
	}

	// ── Managed services decision ─────────────────────────────────────
	for _, svc := range profile.RequiredServices {
		spec.ManagedServices = append(spec.ManagedServices, managedServiceSpec(svc, prefs))
	}

	// ── Object storage decision ───────────────────────────────────────
	if profile.NeedsObjectStorage {
		spec.Storage = append(spec.Storage, StorageSpec{
			Type:       "s3",
			BucketName: fmt.Sprintf("%s-assets-%s", spec.ProjectName, randomSuffix()),
			EnvVarName: "S3_BUCKET_NAME",
		})
	}

	// ── File storage decision ─────────────────────────────────────────
	if profile.NeedsFileStorage {
		spec.Storage = append(spec.Storage, StorageSpec{
			Type:      "efs",
			MountPath: "/data",
		})
	}

	// ── Networking ────────────────────────────────────────────────────
	needsNAT := len(spec.ManagedServices) > 0 || !profile.IsStaticSite
	spec.Network = NetworkSpec{
		CreateVPC:        true,
		VPCCidr:          "10.0.0.0/16",
		PublicSubnets:    2,
		PrivateSubnets:   2,
		EnableNATGateway: needsNAT,
	}

	// ── SSL/TLS ───────────────────────────────────────────────────────
	if spec.LoadBalancer != nil {
		spec.SSL = &SSLSpec{
			Provider:     "acm",
			AutoValidate: true,
			CustomDomain: prefs.CustomDomain,
		}
	}

	return spec
}

// managedServiceSpec converts a RequiredService into a ManagedServiceSpec
func managedServiceSpec(svc RequiredService, prefs UserPreferences) ManagedServiceSpec {
	switch svc.ServiceType {
	case "postgresql":
		return ManagedServiceSpec{
			Type:          "rds",
			Engine:        "postgres",
			EngineVersion: coalesce(svc.Version, "15"),
			InstanceClass: dbInstanceFromPrefs(prefs),
			MultiAZ:       prefs.HighAvailability,
			StorageGB:     20,
			EnvVarName:    coalesce(svc.EnvVarName, "DATABASE_URL"),
		}
	case "mysql":
		return ManagedServiceSpec{
			Type:          "rds",
			Engine:        "mysql",
			EngineVersion: coalesce(svc.Version, "8.0"),
			InstanceClass: dbInstanceFromPrefs(prefs),
			MultiAZ:       prefs.HighAvailability,
			StorageGB:     20,
			EnvVarName:    coalesce(svc.EnvVarName, "DATABASE_URL"),
		}
	case "mongodb":
		return ManagedServiceSpec{
			Type:          "documentdb",
			EngineVersion: coalesce(svc.Version, "6.0"),
			InstanceClass: "db.t3.medium",
			EnvVarName:    coalesce(svc.EnvVarName, "MONGODB_URL"),
		}
	case "redis":
		return ManagedServiceSpec{
			Type:          "elasticache",
			Engine:        "redis",
			EngineVersion: coalesce(svc.Version, "7.1"),
			NodeType:      "cache.t3.micro",
			EnvVarName:    coalesce(svc.EnvVarName, "REDIS_URL"),
		}
	case "rabbitmq":
		return ManagedServiceSpec{
			Type:          "amazon_mq",
			Engine:        "rabbitmq",
			EngineVersion: coalesce(svc.Version, "3.12"),
			InstanceType:  "mq.t3.micro",
			EnvVarName:    coalesce(svc.EnvVarName, "RABBITMQ_URL"),
		}
	case "kafka":
		return ManagedServiceSpec{
			Type:         "msk",
			BrokerNodes:  3,
			InstanceType: "kafka.t3.small",
			EnvVarName:   coalesce(svc.EnvVarName, "KAFKA_BROKERS"),
		}
	case "opensearch":
		return ManagedServiceSpec{
			Type:          "opensearch",
			EngineVersion: coalesce(svc.Version, "2.11"),
			InstanceType:  "t3.small.search",
			EnvVarName:    coalesce(svc.EnvVarName, "OPENSEARCH_URL"),
		}
	default:
		return ManagedServiceSpec{
			Type:       svc.ServiceType,
			EnvVarName: svc.EnvVarName,
		}
	}
}
