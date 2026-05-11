#!/bin/bash
# GitHub Secrets Setup Helper Script
# This script helps you set up GitHub secrets for the CI/CD pipeline

set -e

echo "🔐 AutoStack GitHub Secrets Setup Helper"
echo "=========================================="
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if gh CLI is installed
if ! command -v gh &> /dev/null; then
    echo -e "${RED}❌ GitHub CLI (gh) is not installed${NC}"
    echo "Please install it from: https://cli.github.com/"
    echo ""
    echo "Installation:"
    echo "  macOS:   brew install gh"
    echo "  Linux:   See https://github.com/cli/cli/blob/trunk/docs/install_linux.md"
    exit 1
fi

# Check if logged in
if ! gh auth status &> /dev/null; then
    echo -e "${YELLOW}⚠️  Not logged in to GitHub${NC}"
    echo "Please run: gh auth login"
    exit 1
fi

echo -e "${GREEN}✓ GitHub CLI is installed and authenticated${NC}"
echo ""

# Get repository info
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null || echo "")
if [ -z "$REPO" ]; then
    echo -e "${RED}❌ Not in a GitHub repository${NC}"
    exit 1
fi

echo -e "${BLUE}Repository: $REPO${NC}"
echo ""

# Function to set secret
set_secret() {
    local name=$1
    local description=$2
    local value=$3
    
    if [ -z "$value" ]; then
        echo -e "${YELLOW}⏭️  Skipping $name (no value provided)${NC}"
        return
    fi
    
    echo -e "${BLUE}Setting secret: $name${NC}"
    echo "$value" | gh secret set "$name" --repo "$REPO"
    echo -e "${GREEN}✓ $name set successfully${NC}"
    echo ""
}

# Function to set environment secret
set_env_secret() {
    local env=$1
    local name=$2
    local value=$3
    
    if [ -z "$value" ]; then
        echo -e "${YELLOW}⏭️  Skipping $env/$name (no value provided)${NC}"
        return
    fi
    
    echo -e "${BLUE}Setting environment secret: $env/$name${NC}"
    echo "$value" | gh secret set "$name" --env "$env" --repo "$REPO"
    echo -e "${GREEN}✓ $env/$name set successfully${NC}"
    echo ""
}

echo "📋 Step 1: Kubernetes Configuration"
echo "===================================="
echo ""

# Check for kubeconfig
if [ ! -f ~/.kube/config ]; then
    echo -e "${RED}❌ No kubeconfig found at ~/.kube/config${NC}"
    echo "Please configure kubectl first"
    exit 1
fi

echo "Available Kubernetes contexts:"
kubectl config get-contexts -o name
echo ""

# Staging kubeconfig
echo -e "${YELLOW}Enter staging context name (or press Enter to skip):${NC}"
read -r STAGING_CONTEXT

if [ -n "$STAGING_CONTEXT" ]; then
    echo "Switching to staging context..."
    kubectl config use-context "$STAGING_CONTEXT" || {
        echo -e "${RED}❌ Failed to switch to context: $STAGING_CONTEXT${NC}"
        exit 1
    }
    
    echo "Testing connection..."
    if kubectl cluster-info &> /dev/null; then
        echo -e "${GREEN}✓ Connected to staging cluster${NC}"
        
        echo "Generating base64-encoded kubeconfig..."
        KUBECONFIG_STAGING=$(cat ~/.kube/config | base64 -w 0 2>/dev/null || cat ~/.kube/config | base64)
        
        set_env_secret "staging" "KUBECONFIG_STAGING" "$KUBECONFIG_STAGING"
    else
        echo -e "${RED}❌ Cannot connect to staging cluster${NC}"
    fi
fi

echo ""

# Production kubeconfig
echo -e "${YELLOW}Enter production context name (or press Enter to skip):${NC}"
read -r PROD_CONTEXT

if [ -n "$PROD_CONTEXT" ]; then
    echo "Switching to production context..."
    kubectl config use-context "$PROD_CONTEXT" || {
        echo -e "${RED}❌ Failed to switch to context: $PROD_CONTEXT${NC}"
        exit 1
    }
    
    echo "Testing connection..."
    if kubectl cluster-info &> /dev/null; then
        echo -e "${GREEN}✓ Connected to production cluster${NC}"
        
        echo "Generating base64-encoded kubeconfig..."
        KUBECONFIG_PRODUCTION=$(cat ~/.kube/config | base64 -w 0 2>/dev/null || cat ~/.kube/config | base64)
        
        set_env_secret "production" "KUBECONFIG_PRODUCTION" "$KUBECONFIG_PRODUCTION"
    else
        echo -e "${RED}❌ Cannot connect to production cluster${NC}"
    fi
fi

echo ""
echo "📋 Step 2: Optional Secrets"
echo "============================"
echo ""

# Slack webhook
echo -e "${YELLOW}Enter Slack webhook URL (or press Enter to skip):${NC}"
read -r SLACK_WEBHOOK

if [ -n "$SLACK_WEBHOOK" ]; then
    set_secret "SLACK_WEBHOOK_URL" "Slack webhook for notifications" "$SLACK_WEBHOOK"
fi

echo ""
echo "✅ Setup Complete!"
echo "=================="
echo ""
echo "Next steps:"
echo "1. Create environments in GitHub:"
echo "   - Go to Settings → Environments"
echo "   - Create 'staging' environment (no protection rules)"
echo "   - Create 'production' environment (add required reviewers)"
echo ""
echo "2. Configure branch protection:"
echo "   - Run: ./scripts/setup-branch-protection.sh"
echo ""
echo "3. Test the pipeline:"
echo "   - Create a test PR to develop"
echo "   - Verify CI runs successfully"
echo ""
