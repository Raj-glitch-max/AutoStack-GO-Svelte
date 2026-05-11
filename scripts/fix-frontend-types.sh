#!/bin/bash
# Script to help identify and fix common TypeScript errors in the frontend

set -e

echo "🔍 AutoStack Frontend Type Checker"
echo "=================================="
echo ""

cd frontend

echo "📦 Installing dependencies..."
npm ci --silent

echo ""
echo "🔎 Running type check..."
echo ""

# Run type check and save output
npm run check 2>&1 | tee ../typescript-errors.log

echo ""
echo "📊 Error Summary:"
echo "----------------"

# Count errors by type
echo "Total errors: $(grep -c "Error:" ../typescript-errors.log || echo 0)"
echo "Total warnings: $(grep -c "Warning:" ../typescript-errors.log || echo 0)"

echo ""
echo "📁 Files with errors:"
grep "Error:" ../typescript-errors.log | cut -d':' -f1 | sort | uniq -c | sort -rn || echo "None"

echo ""
echo "🔧 Common error types:"
echo "  - Implicit 'any' types: $(grep -c "implicitly has an 'any' type" ../typescript-errors.log || echo 0)"
echo "  - 'unknown' types: $(grep -c "is of type 'unknown'" ../typescript-errors.log || echo 0)"
echo "  - Type mismatches: $(grep -c "is not assignable to type" ../typescript-errors.log || echo 0)"
echo "  - Missing exports: $(grep -c "has no exported member" ../typescript-errors.log || echo 0)"
echo "  - Undefined variables: $(grep -c "is not defined" ../typescript-errors.log || echo 0)"

echo ""
echo "📝 Full error log saved to: typescript-errors.log"
echo ""
echo "💡 Next steps:"
echo "  1. Review typescript-errors.log for detailed errors"
echo "  2. Fix errors by adding type annotations"
echo "  3. Run 'npm run check' again to verify"
echo "  4. Run 'npm run build' to test production build"
echo ""
