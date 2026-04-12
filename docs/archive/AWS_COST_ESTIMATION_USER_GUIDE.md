# AWS Cost Estimation - User Guide

## Introduction

The AWS Cost Estimation feature helps you understand and control your AWS deployment costs. This guide explains how to use cost estimates, track actual spending, and respond to cost alerts.

## Table of Contents

1. [Getting Started](#getting-started)
2. [Understanding Cost Estimates](#understanding-cost-estimates)
3. [Tracking Actual Costs](#tracking-actual-costs)
4. [Managing Cost Alerts](#managing-cost-alerts)
5. [Cost Optimization Tips](#cost-optimization-tips)
6. [FAQ](#faq)

## Getting Started

### Prerequisites

- An active account on the platform
- AWS credentials configured (for actual cost tracking)
- A blueprint selected for deployment

### Viewing Your First Cost Estimate

1. Navigate to the deployment creation page
2. Select your desired blueprint (e.g., "Web Application")
3. Choose your AWS region (e.g., "us-east-1")
4. The cost estimate will automatically appear

The estimate shows:
- **Monthly cost range**: Minimum and maximum expected costs
- **Cost breakdown**: Itemized costs by service category
- **Assumptions**: What's included in the calculation
- **Disclaimers**: What's NOT included

## Understanding Cost Estimates

### What is a Cost Range?

Instead of showing a single number, we show a range (e.g., $100 - $175/month) because:

- **Usage varies**: Your actual traffic and data transfer will differ from assumptions
- **AWS pricing changes**: Prices may change between estimate and deployment
- **Configuration differences**: Small changes can impact costs

The range is calculated as:
- **Minimum**: 80% of base estimate (optimistic scenario)
- **Maximum**: 140% of base estimate (conservative scenario)

### Cost Breakdown

Costs are broken down into three main categories:

#### 1. Compute Costs
Services that run your application code:
- **Fargate**: Container runtime (charged per vCPU and memory)
- **Lambda**: Serverless functions (charged per invocation and duration)
- **EC2**: Virtual machines (if used)

**Example:**
```
Fargate: $75.00/month
- 2 vCPU @ $0.04048/hour
- 4GB memory @ $0.004445/hour
- 730 hours/month runtime
```

#### 2. Networking Costs
Services that handle traffic and connectivity:
- **ALB**: Application Load Balancer (fixed hourly + per-LCU charges)
- **NAT Gateway**: Outbound internet access (fixed hourly + data transfer)
- **Data Transfer**: Traffic between services and to internet

**Example:**
```
ALB: $25.00/month
- 1 load balancer @ $0.0225/hour
- 10 LCUs average @ $0.008/LCU-hour
```

#### 3. Storage Costs
Services that store your data:
- **RDS**: Managed database (instance + storage)
- **S3**: Object storage (storage + requests)
- **EBS**: Block storage for EC2 instances

**Example:**
```
RDS: $25.50/month
- db.t3.micro instance @ $0.017/hour
- 20GB storage @ $0.115/GB-month
```

### What's Included vs Excluded

#### ✅ Included in Estimates

- Base compute resources (Fargate, Lambda, EC2)
- Standard networking (ALB, NAT Gateway)
- Database instances and storage (RDS)
- Object storage (S3)
- Container registry (ECR)
- Basic CloudWatch logs

#### ❌ Excluded from Estimates

- **Data transfer overages**: Beyond assumed 10GB/month
- **CloudWatch detailed monitoring**: Custom metrics and dashboards
- **Backup storage**: RDS snapshots and S3 versioning
- **Support costs**: AWS support plans
- **Third-party services**: External APIs, monitoring tools
- **Development/testing**: Non-production environments

### Usage Assumptions

All estimates are based on these default assumptions:

| Assumption | Default Value | Adjustable? |
|------------|---------------|-------------|
| Monthly runtime | 730 hours (24/7) | ❌ No |
| Data transfer | 10GB/month | ✅ Yes (future) |
| Storage size | 20GB | ✅ Yes (future) |
| Request volume | 1M requests/month | ❌ No |
| Availability zones | 1 AZ | ❌ No |

**Note**: Custom assumptions will be supported in a future release.

### Pricing Data Freshness

Cost estimates use cached AWS pricing data that is:
- **Refreshed**: Every 24 hours automatically
- **Region-specific**: Prices vary by AWS region
- **Timestamped**: You can see when data was last fetched

If pricing data is older than 48 hours, you'll see a warning:
```
⚠️ Warning: Pricing data is stale (last updated 3 days ago)
Estimates may not reflect current AWS prices.
```

## Tracking Actual Costs

### Enabling Cost Tracking

To track actual AWS costs, you need to:

1. Configure AWS credentials with Cost Explorer access
2. Deploy your application
3. Wait 48 hours for AWS Cost Explorer data to become available

### Viewing Actual Costs

After deployment, navigate to your deployment detail page to see:

- **Cost to date**: Total spent since deployment
- **Projected monthly**: Estimated full-month cost based on current usage
- **Variance**: Difference between actual and estimated costs
- **Service breakdown**: Which services are costing the most

### Understanding Variance

Variance shows how actual costs compare to estimates:

```
Variance: +23.5% ($17.25 over estimate)
```

**Color coding:**
- 🟢 **Green** (0-10%): Within expected range
- 🟡 **Yellow** (10-20%): Slightly higher than expected
- 🔴 **Red** (>20%): Significantly over estimate

### Common Reasons for Variance

#### Positive Variance (Costs Higher Than Expected)

1. **Higher traffic**: More requests than assumed
2. **Data transfer**: More outbound data than 10GB/month
3. **Storage growth**: Database or S3 storage increased
4. **Additional services**: CloudWatch alarms, SNS notifications, etc.

#### Negative Variance (Costs Lower Than Expected)

1. **Lower traffic**: Fewer requests than assumed
2. **Efficient caching**: Reduced database queries
3. **Optimized resources**: Right-sized compute instances

### Cost Data Delay

AWS Cost Explorer has a 48-hour delay, which means:

- **Day 1-2**: No actual cost data available (shows estimate only)
- **Day 3+**: Actual costs appear with 48-hour lag

During the delay period, you'll see:
```
ℹ️ Actual cost data will be available after April 13, 2026
Currently showing estimated costs only.
```

## Managing Cost Alerts

### What Are Cost Alerts?

Cost alerts notify you when actual costs exceed your estimate by a threshold percentage (default: 20%).

### Alert Severity Levels

- **Info** (0-10% over): Minor variance, no action needed
- **Warning** (10-20% over): Approaching threshold, monitor closely
- **Critical** (>20% over): Exceeded threshold, action recommended

### Receiving Alerts

Alerts are delivered via:
- **Email**: Sent to your registered email address
- **In-app**: Notification badge in the platform UI

### Alert Details

Each alert includes:

1. **Variance information**: How much over estimate
2. **Service breakdown**: Which services exceeded budget
3. **Recommendations**: Suggested actions to reduce costs

**Example Alert:**
```
🔴 Cost Alert: My Web App

Actual cost exceeds estimate by 23.5%
- Estimated: $125.50/month
- Actual: $142.75/month
- Variance: +$17.25

Service Breakdown:
- Fargate: +9.3% ($6.98 over)
- ALB: +14.0% ($3.50 over)
- RDS: +6.8% ($1.86 over)

Recommendations:
✓ Review Fargate task sizing - may be over-provisioned
✓ Check for unexpected traffic spikes in ALB metrics
✓ Consider enabling auto-scaling to optimize costs
```

### Acknowledging Alerts

To acknowledge an alert:

1. Click on the alert in your notifications
2. Review the details and recommendations
3. Click "Acknowledge" button
4. Optionally add a note (e.g., "Investigating traffic spike")

Acknowledged alerts remain visible but won't send repeat notifications.

### Customizing Alert Thresholds

You can customize alert thresholds per deployment:

1. Navigate to deployment settings
2. Go to "Cost Alerts" section
3. Adjust threshold percentage (5% - 50%)
4. Enable/disable email and in-app notifications

**Example:**
```
Default threshold: 20%
Production app: 15% (more sensitive)
Development app: 50% (less sensitive)
```

## Cost Optimization Tips

### 1. Right-Size Your Resources

**Problem**: Over-provisioned Fargate tasks
**Solution**: Monitor CPU and memory usage, reduce if consistently low

```
Current: 2 vCPU, 4GB memory → $75/month
Optimized: 1 vCPU, 2GB memory → $37.50/month
Savings: $37.50/month (50%)
```

### 2. Optimize Data Transfer

**Problem**: High data transfer costs
**Solution**: Enable compression, use CloudFront CDN, optimize API responses

```
Current: 50GB/month → $4.50/month
Optimized: 20GB/month → $1.80/month
Savings: $2.70/month (60%)
```

### 3. Use Auto-Scaling

**Problem**: Running full capacity 24/7
**Solution**: Scale down during low-traffic periods

```
Current: 2 tasks 24/7 → $150/month
Optimized: 2 tasks peak, 1 task off-peak → $112.50/month
Savings: $37.50/month (25%)
```

### 4. Optimize Database Storage

**Problem**: Over-provisioned RDS storage
**Solution**: Start with smaller storage, enable auto-scaling

```
Current: 100GB provisioned → $11.50/month
Optimized: 20GB with auto-scaling → $2.30/month
Savings: $9.20/month (80%)
```

### 5. Clean Up Unused Resources

**Problem**: Orphaned resources after deployment deletion
**Solution**: Regularly audit and delete unused:
- ECR images
- CloudWatch log groups
- S3 buckets
- RDS snapshots

### 6. Use Reserved Capacity (Future)

For long-running production workloads, consider:
- **Fargate Savings Plans**: Up to 50% discount
- **RDS Reserved Instances**: Up to 60% discount

**Note**: Reserved capacity recommendations will be added in a future release.

## FAQ

### Q: Why is my estimate a range instead of a single number?

**A**: Real-world usage varies, and we want to set realistic expectations. The range accounts for:
- Traffic fluctuations
- Data transfer variations
- AWS pricing changes
- Configuration differences

### Q: How accurate are the estimates?

**A**: Our goal is 90% of deployments within the estimated range. Accuracy depends on:
- How closely your usage matches assumptions
- Whether you add additional services
- Regional pricing differences

### Q: Can I get a more precise estimate?

**A**: Future releases will support:
- Custom usage assumptions
- Historical usage data
- Reserved capacity pricing
- Multi-month projections

### Q: Why is there a 48-hour delay for actual costs?

**A**: This is an AWS Cost Explorer limitation. AWS processes billing data with a 48-hour lag to ensure accuracy.

### Q: What if my actual costs are much higher than estimated?

**A**: First, check the service breakdown to identify the source. Common causes:
1. Higher traffic than assumed
2. Additional services not in estimate
3. Data transfer overages
4. Development/testing environments

Then review our [Cost Optimization Tips](#cost-optimization-tips).

### Q: Can I set a hard budget limit?

**A**: Not currently. We provide alerts but don't enforce limits. Future releases may integrate with AWS Budgets for hard limits.

### Q: How do I reduce my AWS costs?

**A**: See our [Cost Optimization Tips](#cost-optimization-tips) section. Key strategies:
- Right-size resources
- Enable auto-scaling
- Optimize data transfer
- Clean up unused resources

### Q: What if pricing data is stale?

**A**: The system continues using cached data with a warning. Estimates may be slightly inaccurate but are still useful for planning.

### Q: Can I export cost data?

**A**: Not currently. Future releases will support:
- CSV export
- Cost reports
- Historical trending
- Budget forecasting

### Q: Do estimates include taxes?

**A**: No. AWS taxes vary by region and customer type. Add 0-20% depending on your location.

### Q: What about multi-region deployments?

**A**: Each region is estimated separately. Total cost = sum of all regions.

## Getting Help

### Support Resources

- **Documentation**: [AWS Cost Estimation API](./AWS_COST_ESTIMATION_API.md)
- **Community**: Platform community forum
- **Support**: Contact platform support team

### Reporting Issues

If you notice:
- Inaccurate estimates
- Missing cost data
- Alert issues
- Performance problems

Please report with:
1. Deployment ID
2. Blueprint used
3. Region selected
4. Screenshot of issue
5. Expected vs actual behavior

## Appendix: AWS Pricing Resources

- [AWS Pricing Calculator](https://calculator.aws/)
- [AWS Cost Management](https://aws.amazon.com/aws-cost-management/)
- [AWS Cost Explorer](https://aws.amazon.com/aws-cost-management/aws-cost-explorer/)
- [AWS Pricing API](https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/price-changes.html)

---

**Last Updated**: April 11, 2026  
**Version**: 1.0.0
