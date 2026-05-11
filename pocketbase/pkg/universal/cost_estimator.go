package universal

// EstimateCost produces a cost estimate from an InfrastructureSpec.
// Uses well-known AWS pricing as of 2025 (us-east-1 baseline).
// Regional multipliers applied for other regions.
func EstimateCost(spec InfrastructureSpec) UniversalCostEstimate {
	breakdown := make(map[string]float64)

	// ── Compute ───────────────────────────────────────────────────────
	switch spec.Compute.Type {
	case ComputeTypeECSFargate:
		// Fargate pricing: vCPU-hour + GB-hour
		// Assume 1 task running 24/7 at 50% utilization
		vcpu := float64(spec.Compute.CPU) / 1024.0
		memGB := float64(spec.Compute.Memory) / 1024.0
		hoursPerMonth := 730.0 * 0.5 // 50% utilization (scales to zero)
		cpuCost := vcpu * hoursPerMonth * 0.04048
		memCost := memGB * hoursPerMonth * 0.004445
		breakdown["ECS Fargate"] = round2(cpuCost + memCost)

	case ComputeTypeS3Static:
		breakdown["S3 Static Hosting"] = 0.50 // minimal

	case ComputeTypeECSEC2:
		// g4dn.xlarge ~$0.526/hr
		breakdown["ECS EC2 (GPU)"] = round2(0.526 * 730)
	}

	// ── Load balancer ─────────────────────────────────────────────────
	if spec.LoadBalancer != nil {
		// ALB: $0.008/hr + $0.008/LCU-hr (assume 5 LCUs)
		breakdown["Application Load Balancer"] = round2(0.008*730 + 0.008*5*730)
	}

	// ── CDN ───────────────────────────────────────────────────────────
	if spec.CDN != nil {
		// CloudFront: ~$1/month for low traffic
		breakdown["CloudFront CDN"] = 1.0
	}

	// ── Managed services ──────────────────────────────────────────────
	for _, svc := range spec.ManagedServices {
		switch svc.Type {
		case "rds":
			// db.t3.micro: ~$0.017/hr for postgres
			hourlyRate := 0.017
			if svc.InstanceClass == "db.t3.small" {
				hourlyRate = 0.034
			}
			instanceCost := hourlyRate * 730
			storageCost := float64(svc.StorageGB) * 0.115 // gp2 storage
			label := "RDS " + svc.Engine + " (" + svc.InstanceClass + ")"
			if svc.MultiAZ {
				instanceCost *= 2
				label += " Multi-AZ"
			}
			breakdown[label] = round2(instanceCost + storageCost)

		case "elasticache":
			// cache.t3.micro: ~$0.017/hr
			breakdown["ElastiCache Redis (cache.t3.micro)"] = round2(0.017 * 730)

		case "documentdb":
			// db.t3.medium: ~$0.078/hr
			breakdown["DocumentDB (db.t3.medium)"] = round2(0.078 * 730)

		case "amazon_mq":
			// mq.t3.micro: ~$0.027/hr
			breakdown["Amazon MQ RabbitMQ (mq.t3.micro)"] = round2(0.027 * 730)

		case "msk":
			// kafka.t3.small × 3 brokers: ~$0.021/hr each
			breakdown["MSK Kafka (3 × kafka.t3.small)"] = round2(0.021 * 3 * 730)

		case "opensearch":
			// t3.small.search: ~$0.036/hr
			breakdown["OpenSearch (t3.small.search)"] = round2(0.036 * 730)
		}
	}

	// ── Storage ───────────────────────────────────────────────────────
	for _, s := range spec.Storage {
		switch s.Type {
		case "s3":
			breakdown["S3 Object Storage"] = 0.50 // minimal for new bucket
		case "efs":
			// EFS: $0.30/GB-month, assume 10GB
			breakdown["EFS File Storage"] = round2(0.30 * 10)
		}
	}

	// ── NAT Gateway ───────────────────────────────────────────────────
	if spec.Network.EnableNATGateway {
		// NAT Gateway: $0.045/hr + data processing
		breakdown["NAT Gateway"] = round2(0.045 * 730)
	}

	// ── Apply regional multiplier ─────────────────────────────────────
	multiplier := regionalMultiplier(spec.Region)
	total := 0.0
	for k, v := range breakdown {
		adjusted := round2(v * multiplier)
		breakdown[k] = adjusted
		total += adjusted
	}

	total = round2(total)

	return UniversalCostEstimate{
		MonthlyMin:      round2(total * 0.8),
		MonthlyEstimate: total,
		MonthlyMax:      round2(total * 1.5),
		Currency:        "USD",
		Breakdown:       breakdown,
	}
}

// regionalMultiplier returns a cost multiplier for a given AWS region
func regionalMultiplier(region string) float64 {
	m := map[string]float64{
		"us-east-1":      1.00,
		"us-east-2":      1.00,
		"us-west-1":      1.10,
		"us-west-2":      1.00,
		"eu-west-1":      1.10,
		"eu-west-2":      1.15,
		"eu-central-1":   1.12,
		"ap-southeast-1": 1.15,
		"ap-northeast-1": 1.20,
		"ap-south-1":     1.05,
		"sa-east-1":      1.25,
	}
	if m, ok := m[region]; ok {
		return m
	}
	return 1.10 // conservative default for unknown regions
}

// round2 rounds to 2 decimal places
func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
