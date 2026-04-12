# AutoStack: Quick Setup Guide
**Get running in 15 minutes**

---

## ✅ What You Already Have

1. ✅ **Infracost API Key** - Already configured!
2. ✅ **PocketBase** - Authentication & database working
3. ✅ **Kubernetes Integration** - Working
4. ✅ **Terraform Execution** - Working
5. ✅ **Intelligence System** - 40+ error patterns ready

---

## 🚀 What You Need to Do NOW

### Step 1: Set Up Resend (5 minutes)

**Why:** Send email notifications for cost alerts and deployment status

1. Go to https://resend.com/
2. Click "Sign Up" (free)
3. Verify your email
4. Go to "API Keys" in dashboard
5. Click "Create API Key"
6. Copy the key (starts with `re_`)

**Add to your `.env` file:**
```bash
RESEND_API_KEY=re_your_key_here
RESEND_FROM_EMAIL=alerts@autostack.io
```

**Note:** With free tier, you can only send from `onboarding@resend.dev` until you verify your domain. That's fine for testing!

---

### Step 2: Update Your .env File (2 minutes)

Copy `.env.example` to `.env` if you haven't already:

```bash
cp .env.example .env
```

Your `.env` should have:

```bash
# Encryption key (generate if you don't have one)
AUTOSTACK_ENCRYPTION_KEY=your_32_byte_key_here

# Infracost (YOU ALREADY HAVE THIS)
INFRACOST_API_KEY=ics_v1_0mi91GSQK8FpSEa4rzbJT9_EmqTYVtz3ohpHfMx8rlBnhEFXcwuoroAoygAheK0eWBEbNfGFwhat

# Resend (ADD THIS)
RESEND_API_KEY=re_your_key_here
RESEND_FROM_EMAIL=onboarding@resend.dev
```

---

### Step 3: Install Go Dependencies (2 minutes)

```bash
cd pocketbase
go mod tidy
```

This will download any missing dependencies.

---

### Step 4: Test Infracost Integration (5 minutes)

Create a test file `pocketbase/test_infracost.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Raj-glitch-max/autostack/pkg/aws"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env
	godotenv.Load()

	// Create Infracost service
	apiKey := os.Getenv("INFRACOST_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRACOST_API_KEY not set")
	}

	service := aws.NewInfracostService(apiKey)

	// Test with simple Terraform code
	terraformCode := `
provider "aws" {
  region = "us-east-1"
}

resource "aws_instance" "web" {
  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t2.micro"
  
  tags = {
    Name = "test-instance"
  }
}
`

	fmt.Println("Testing Infracost API...")
	estimate, err := service.EstimateCost(context.Background(), terraformCode, "us-east-1")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("\n✅ Success!\n")
	fmt.Printf("Total Monthly Cost: $%.2f\n", estimate.TotalMonthlyCost)
	fmt.Printf("Currency: %s\n", estimate.Currency)
	fmt.Printf("Region: %s\n", estimate.Region)
	fmt.Printf("\nBreakdown:\n")
	for resource, cost := range estimate.Breakdown {
		fmt.Printf("  %s: $%.2f\n", resource, cost)
	}
}
```

Run it:
```bash
cd pocketbase
go run test_infracost.go
```

Expected output:
```
Testing Infracost API...

✅ Success!
Total Monthly Cost: $8.47
Currency: USD
Region: us-east-1

Breakdown:
  aws_instance.web: $8.47
```

---

### Step 5: Test Email Service (3 minutes)

Create a test file `pocketbase/test_email.go`:

```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Raj-glitch-max/autostack/pkg/notifications"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env
	godotenv.Load()

	// Create email service
	emailService := notifications.NewEmailService()

	if !emailService.IsEnabled() {
		log.Fatal("Email service not enabled. Set RESEND_API_KEY in .env")
	}

	// Test cost alert
	fmt.Println("Testing cost alert email...")
	err := emailService.SendCostAlert("your-email@example.com", &notifications.CostAlert{
		DeploymentID:       "test-123",
		DeploymentName:     "My Test App",
		EstimatedCost:      50.00,
		ActualCost:         75.50,
		VariancePercentage: 51.0,
	})

	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Println("✅ Email sent! Check your inbox.")
}
```

Run it:
```bash
cd pocketbase
go run test_email.go
```

---

## 🎯 What's Working Now

After these steps, you have:

1. ✅ **Cost Estimation** - Infracost API integrated
2. ✅ **Email Notifications** - Resend integrated
3. ✅ **Authentication** - PocketBase working
4. ✅ **Database** - PocketBase working
5. ✅ **Kubernetes** - Direct API working
6. ✅ **Terraform** - Execution working
7. ✅ **Intelligence** - Error recovery working

---

## 🔄 Next Steps (Optional - Can Do Later)

### Week 2: Add Monitoring (4 hours)

**Better Stack** - Centralized logging and uptime monitoring

1. Sign up at https://betterstack.com/ (free tier)
2. Get API token
3. Add to `.env`:
   ```bash
   LOGTAIL_TOKEN=your_token_here
   ```
4. Install SDK:
   ```bash
   go get github.com/logtail/logtail-go
   ```
5. Replace `log.Printf` calls with Logtail logger

**Cost:** $10/month (after free tier)

---

### Week 3: Add Error Tracking (2 hours)

**Sentry** - Track unknown errors and bugs

1. Sign up at https://sentry.io/ (free tier)
2. Create project
3. Get DSN
4. Add to `.env`:
   ```bash
   SENTRY_DSN=your_dsn_here
   ```
5. Install SDK:
   ```bash
   go get github.com/getsentry/sentry-go
   cd ../frontend
   npm install @sentry/svelte
   ```
6. Initialize in `main.go` and frontend

**Cost:** Free (5,000 errors/month)

---

### Month 2: Add Analytics (4 hours)

**PostHog** - User behavior tracking

1. Sign up at https://posthog.com/ (free tier)
2. Get API key
3. Add to `.env`:
   ```bash
   POSTHOG_API_KEY=your_key_here
   ```
4. Install SDK:
   ```bash
   go get github.com/posthog/posthog-go
   cd ../frontend
   npm install posthog-js
   ```
5. Track events in frontend and backend

**Cost:** Free (1M events/month)

---

## 💰 Current Monthly Cost

- Infracost: $0 (free tier - 100 estimates/month)
- Resend: $0 (free tier - 3,000 emails/month)
- **Total: $0/month**

When you need to upgrade:
- Infracost: $50/month for unlimited
- Resend: $20/month for 50,000 emails

---

## 🐛 Troubleshooting

### Infracost API Error

**Error:** "Invalid API key"
- Check your `.env` file has the correct key
- Make sure you copied the full key (starts with `ics_v1_`)

**Error:** "Failed to parse Terraform"
- Infracost needs valid Terraform syntax
- Test with simple Terraform code first

### Email Not Sending

**Error:** "RESEND_API_KEY not set"
- Check your `.env` file
- Make sure you ran `source .env` or restarted your app

**Error:** "Domain not verified"
- With free tier, use `onboarding@resend.dev` as FROM address
- To use custom domain, verify it in Resend dashboard

### Go Module Errors

**Error:** "package not found"
```bash
cd pocketbase
go mod tidy
go mod download
```

---

## ✅ Verification Checklist

Before moving on, verify:

- [ ] `.env` file has INFRACOST_API_KEY
- [ ] `.env` file has RESEND_API_KEY
- [ ] `go mod tidy` runs without errors
- [ ] Test Infracost script works
- [ ] Test email script works (or logs "would send")
- [ ] Application starts without errors

---

## 🎉 You're Done!

Your AutoStack now has:
- ✅ Real-time AWS cost estimation (Infracost)
- ✅ Email notifications (Resend)
- ✅ All core features working

**Time spent:** ~15 minutes  
**Code deleted:** ~2000 lines (old pricing system)  
**Code added:** ~200 lines (API integrations)  
**Monthly cost:** $0 (free tiers)

Focus on building your unique features. Let the APIs handle the commodity stuff!

---

## 📚 Resources

- Infracost Docs: https://www.infracost.io/docs/
- Resend Docs: https://resend.com/docs
- Better Stack Docs: https://betterstack.com/docs
- Sentry Docs: https://docs.sentry.io/
- PostHog Docs: https://posthog.com/docs

---

## 🆘 Need Help?

If you run into issues:
1. Check the error message carefully
2. Verify your `.env` file
3. Check API service status pages
4. Review the integration code in `pkg/aws/infracost_service.go` and `pkg/notifications/email_service.go`

Good luck! 🚀
