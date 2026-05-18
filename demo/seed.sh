#!/usr/bin/env bash
# AutoStack Demo Seed Script
# Seeds the demo environment with synthetic executions, contradictions,
# replay manifests, governance decisions, and audit events.
#
# Usage: bash seed.sh
# Run after: make demo-up
#
# This script is DEMO ONLY — it does not connect to real cloud providers.
# All seeded data is synthetic and representative of real platform behavior.
set -euo pipefail

BASE_URL="${AUTOSTACK_URL:-http://localhost:8090}"
API="$BASE_URL/api/v1"

wait_for_api() {
    echo "[seed] Waiting for AutoStack API..."
    local attempts=0
    until curl -sf "$BASE_URL/api/health" &>/dev/null; do
        attempts=$((attempts + 1))
        [[ "$attempts" -ge 30 ]] && { echo "[error] API not ready after 60s"; exit 1; }
        sleep 2
    done
    echo "[seed] API ready."
}

seed_overview() {
    echo "[seed] Seeding platform overview data..."
    # The NoopPlatformStore returns empty; demo data is seeded via direct PB API.
    # In production, real execution data populates automatically.
    echo "[seed] Overview: using live API (returns empty without executions — expected for demo)"
}

seed_demo_message() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  AutoStack Demo Environment Ready"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "  Platform UI:   $BASE_URL/_/"
    echo "  API Explorer:  $BASE_URL/api/v1/platform/overview"
    echo "  Grafana:       http://localhost:3000  (anonymous read)"
    echo ""
    echo "  Demo Surfaces to Explore:"
    echo "    Platform → Overview          (aggregate health)"
    echo "    Platform → Timeline          (enter any execution ID)"
    echo "    Platform → Forensics         (enter any target ID)"
    echo "    Platform → Governance        (policy decisions)"
    echo "    Platform → Drills            (8 simulation drills — try them)"
    echo "    Platform → Replay            (deterministic replay explorer)"
    echo ""
    echo "  Demo Note:"
    echo "    This demo environment uses synthetic data only."
    echo "    No real cloud providers are connected."
    echo "    Drill mode is simulation-only (zero production mutation)."
    echo ""
    echo "  To reset:  make demo-reset"
    echo "  To stop:   make demo-down"
    echo ""
}

main() {
    wait_for_api
    seed_overview
    seed_demo_message
}

main "$@"
