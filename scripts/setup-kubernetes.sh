#!/bin/bash
# Kubernetes Namespace Setup Script
# Creates namespaces and verifies cluster connectivity

set -e

echo "☸️  AutoStack Kubernetes Setup"
echo "=============================="
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if kubectl is installed
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}❌ kubectl is not installed${NC}"
    echo "Please install kubectl first"
    exit 1
fi

echo -e "${GREEN}✓ kubectl is installed${NC}"
echo ""

# Show available contexts
echo "Available Kubernetes contexts:"
kubectl config get-contexts -o name
echo ""

# Function to setup namespace
setup_namespace() {
    local context=$1
    local namespace=$2
    local env_name=$3
    
    echo -e "${BLUE}Setting up $env_name environment${NC}"
    echo "Context: $context"
    echo "Namespace: $namespace"
    echo ""
    
    # Switch context
    echo "Switching to context: $context"
    if ! kubectl config use-context "$context"; then
        echo -e "${RED}❌ Failed to switch to context: $context${NC}"
        return 1
    fi
    
    # Test connection
    echo "Testing cluster connection..."
    if ! kubectl cluster-info &> /dev/null; then
        echo -e "${RED}❌ Cannot connect to cluster${NC}"
        return 1
    fi
    echo -e "${GREEN}✓ Connected to cluster${NC}"
    
    # Create namespace
    echo "Creating namespace: $namespace"
    if kubectl get namespace "$namespace" &> /dev/null; then
        echo -e "${YELLOW}⚠️  Namespace $namespace already exists${NC}"
    else
        kubectl create namespace "$namespace"
        echo -e "${GREEN}✓ Namespace $namespace created${NC}"
    fi
    
    # Verify namespace
    echo "Verifying namespace..."
    if kubectl get namespace "$namespace" -o jsonpath='{.status.phase}' | grep -q "Active"; then
        echo -e "${GREEN}✓ Namespace $namespace is Active${NC}"
    else
        echo -e "${RED}❌ Namespace $namespace is not Active${NC}"
        return 1
    fi
    
    # Show namespace info
    echo ""
    echo "Namespace details:"
    kubectl get namespace "$namespace"
    echo ""
    
    return 0
}

# Setup staging
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📦 Staging Environment Setup"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo -e "${YELLOW}Enter staging context name (or press Enter to skip):${NC}"
read -r STAGING_CONTEXT

if [ -n "$STAGING_CONTEXT" ]; then
    if setup_namespace "$STAGING_CONTEXT" "autostack-staging" "Staging"; then
        echo -e "${GREEN}✅ Staging environment setup complete${NC}"
    else
        echo -e "${RED}❌ Staging environment setup failed${NC}"
    fi
else
    echo -e "${YELLOW}⏭️  Skipping staging setup${NC}"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🚀 Production Environment Setup"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo -e "${YELLOW}Enter production context name (or press Enter to skip):${NC}"
read -r PROD_CONTEXT

if [ -n "$PROD_CONTEXT" ]; then
    if setup_namespace "$PROD_CONTEXT" "autostack-production" "Production"; then
        echo -e "${GREEN}✅ Production environment setup complete${NC}"
    else
        echo -e "${RED}❌ Production environment setup failed${NC}"
    fi
else
    echo -e "${YELLOW}⏭️  Skipping production setup${NC}"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Kubernetes Setup Complete"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Summary:"
if [ -n "$STAGING_CONTEXT" ]; then
    echo -e "${GREEN}✓ Staging namespace: autostack-staging${NC}"
fi
if [ -n "$PROD_CONTEXT" ]; then
    echo -e "${GREEN}✓ Production namespace: autostack-production${NC}"
fi

echo ""
echo "Next steps:"
echo "1. Set up GitHub secrets:"
echo "   ./scripts/setup-github-secrets.sh"
echo ""
echo "2. Configure GitHub environments:"
echo "   - Go to Settings → Environments"
echo "   - Create 'staging' and 'production' environments"
echo ""
echo "3. Test deployments:"
echo "   kubectl get pods -n autostack-staging"
echo "   kubectl get pods -n autostack-production"
echo ""
