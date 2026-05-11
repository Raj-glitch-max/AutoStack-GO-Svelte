#!/bin/bash
# CI/CD Setup Verification Script
# Checks if all prerequisites are met for the CI/CD pipeline

set -e

echo "🔍 AutoStack CI/CD Setup Verification"
echo "======================================"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

ERRORS=0
WARNINGS=0

# Function to check and report
check() {
    local name=$1
    local command=$2
    local required=$3
    
    echo -n "Checking $name... "
    if eval "$command" &> /dev/null; then
        echo -e "${GREEN}✓${NC}"
        return 0
    else
        if [ "$required" = "true" ]; then
            echo -e "${RED}✗ (Required)${NC}"
            ((ERRORS++))
        else
            echo -e "${YELLOW}⚠ (Optional)${NC}"
            ((WARNINGS++))
        fi
        return 1
    fi
}

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📦 Local Environment"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

check "Git" "command -v git" "true"
check "Docker" "command -v docker" "true"
check "kubectl" "command -v kubectl" "true"
check "GitHub CLI (gh)" "command -v gh" "false"
check "Go 1.22+" "go version | grep -E 'go1\.(2[2-9]|[3-9][0-9])'" "true"
check "Node.js 20+" "node --version | grep -E 'v(2[0-9]|[3-9][0-9])'" "true"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🏗️  Code Quality"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Backend checks
echo "Backend (Go):"
cd pocketbase 2>/dev/null || { echo -e "${RED}✗ pocketbase directory not found${NC}"; ((ERRORS++)); }

if [ -d "pocketbase" ]; then
    check "  Go modules verified" "cd pocketbase && go mod verify" "true"
    check "  Go vet passes" "cd pocketbase && go vet ./..." "true"
    check "  Go build succeeds" "cd pocketbase && go build -o /tmp/autostack-test ./..." "true"
    rm -f /tmp/autostack-test
fi

cd ..

# Frontend checks
echo ""
echo "Frontend (SvelteKit):"
if [ -d "frontend" ]; then
    check "  npm dependencies" "cd frontend && npm ci --silent" "true"
    check "  TypeScript check" "cd frontend && npm run check" "true"
    check "  Frontend build" "cd frontend && npm run build" "true"
else
    echo -e "${RED}✗ frontend directory not found${NC}"
    ((ERRORS++))
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📄 Configuration Files"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

check "Dockerfile" "test -f Dockerfile" "true"
check ".golangci.yml" "test -f .golangci.yml" "true"
check ".tflint.hcl" "test -f .tflint.hcl" "true"
check "CI workflow" "test -f .github/workflows/ci.yml" "true"
check "CD workflow" "test -f .github/workflows/cd.yml" "true"
check "Security workflow" "test -f .github/workflows/security.yml" "true"
check "Terraform validate workflow" "test -f .github/workflows/terraform-validate.yml" "true"
check "Release workflow" "test -f .github/workflows/release.yml" "true"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "☸️  Kubernetes"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if command -v kubectl &> /dev/null; then
    check "kubectl configured" "test -f ~/.kube/config" "true"
    
    if [ -f ~/.kube/config ]; then
        echo "Available contexts:"
        kubectl config get-contexts -o name | sed 's/^/  - /'
        echo ""
        
        # Check for staging namespace
        CURRENT_CONTEXT=$(kubectl config current-context)
        echo "Current context: $CURRENT_CONTEXT"
        
        if kubectl get namespace autostack-staging &> /dev/null; then
            echo -e "${GREEN}✓ autostack-staging namespace exists${NC}"
        else
            echo -e "${YELLOW}⚠ autostack-staging namespace not found${NC}"
            ((WARNINGS++))
        fi
        
        if kubectl get namespace autostack-production &> /dev/null; then
            echo -e "${GREEN}✓ autostack-production namespace exists${NC}"
        else
            echo -e "${YELLOW}⚠ autostack-production namespace not found${NC}"
            ((WARNINGS++))
        fi
    fi
else
    echo -e "${RED}✗ kubectl not installed${NC}"
    ((ERRORS++))
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🐙 GitHub"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if command -v gh &> /dev/null; then
    check "GitHub CLI authenticated" "gh auth status" "false"
    
    if gh auth status &> /dev/null; then
        REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null || echo "")
        if [ -n "$REPO" ]; then
            echo -e "${GREEN}✓ Repository: $REPO${NC}"
            
            # Check secrets (requires admin access)
            echo ""
            echo "Checking GitHub secrets (requires admin access)..."
            if gh secret list &> /dev/null; then
                SECRETS=$(gh secret list 2>/dev/null | wc -l)
                echo "  Found $SECRETS repository secrets"
            else
                echo -e "${YELLOW}  ⚠ Cannot list secrets (may need admin access)${NC}"
            fi
        else
            echo -e "${YELLOW}⚠ Not in a GitHub repository${NC}"
            ((WARNINGS++))
        fi
    fi
else
    echo -e "${YELLOW}⚠ GitHub CLI not installed (optional)${NC}"
    ((WARNINGS++))
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 Summary"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
    echo -e "${GREEN}✅ All checks passed!${NC}"
    echo ""
    echo "Your CI/CD pipeline is ready to use."
    echo ""
    echo "Next steps:"
    echo "1. Push to develop branch to trigger CI"
    echo "2. Merge to develop to deploy to staging"
    echo "3. Create PR to main for production deployment"
elif [ $ERRORS -eq 0 ]; then
    echo -e "${YELLOW}⚠️  $WARNINGS warning(s) found${NC}"
    echo ""
    echo "The pipeline should work, but some optional features may not be available."
    echo ""
    echo "To fix warnings:"
    echo "- Install GitHub CLI: brew install gh (macOS) or see https://cli.github.com/"
    echo "- Create Kubernetes namespaces: ./scripts/setup-kubernetes.sh"
else
    echo -e "${RED}❌ $ERRORS error(s) and $WARNINGS warning(s) found${NC}"
    echo ""
    echo "Please fix the errors before using the CI/CD pipeline."
    echo ""
    echo "Common fixes:"
    echo "- Install missing tools (Go, Node.js, Docker, kubectl)"
    echo "- Run 'go mod tidy' in pocketbase directory"
    echo "- Run 'npm install' in frontend directory"
    echo "- Create Kubernetes namespaces: ./scripts/setup-kubernetes.sh"
    exit 1
fi

echo ""
echo "Documentation:"
echo "- Setup guide: CI_CD_SETUP_CHECKLIST.md"
echo "- Workflow guide: docs/DEPLOYMENT_WORKFLOW.md"
echo "- Status report: FINAL_CI_CD_STATUS.md"
echo ""
