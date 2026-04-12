// +build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/notifications"
	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("🚀 AutoStack Integration Test")
	fmt.Println("================================\n")

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Test 1: Infracost Service
	fmt.Println("Test 1: Infracost API Integration")
	fmt.Println("----------------------------------")
	testInfracost()

	fmt.Println()

	// Test 2: Email Service
	fmt.Println("Test 2: Resend Email Integration")
	fmt.Println("----------------------------------")
	testEmail()

	fmt.Println()
	fmt.Println("✅ All tests completed!")
}

func testInfracost() {
	apiKey := os.Getenv("INFRACOST_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ INFRACOST_API_KEY not set")
		return
	}

	fmt.Printf("✓ API Key configured (length: %d)\n", len(apiKey))

	// Create service
	service := aws.NewInfracostService(apiKey)
	fmt.Println("✓ Infracost service created")

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

	fmt.Println("✓ Testing cost estimation...")
	estimate, err := service.EstimateCost(context.Background(), terraformCode, "us-east-1")
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}

	fmt.Printf("✓ Cost estimation successful!\n")
	fmt.Printf("  Total Monthly Cost: $%.2f\n", estimate.TotalMonthlyCost)
	fmt.Printf("  Currency: %s\n", estimate.Currency)
	fmt.Printf("  Region: %s\n", estimate.Region)
	
	if len(estimate.Breakdown) > 0 {
		fmt.Println("  Breakdown:")
		for resource, cost := range estimate.Breakdown {
			fmt.Printf("    - %s: $%.2f\n", resource, cost)
		}
	}
}

func testEmail() {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ RESEND_API_KEY not set")
		return
	}

	fmt.Printf("✓ API Key configured (length: %d)\n", len(apiKey))

	// Create service
	service := notifications.NewEmailService()
	fmt.Println("✓ Email service created")

	if !service.IsEnabled() {
		fmt.Println("❌ Email service not enabled")
		return
	}

	fmt.Println("✓ Email service enabled")
	fmt.Println("✓ Ready to send notifications")
	
	// Note: We don't actually send an email in the test
	// to avoid spamming. The service is ready to use.
	fmt.Println("\n  To send a test email, use:")
	fmt.Println("  service.SendCostAlert(\"your-email@example.com\", &notifications.CostAlertData{...})")
}
