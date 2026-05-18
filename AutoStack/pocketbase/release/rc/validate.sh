#!/usr/bin/env bash
# AutoStack Release Candidate Validation Script
# Verifies build, tests, vet, and benchmark smoke run before tagging.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)/AutoStack/pocketbase"

echo "=== AutoStack RC Validation ==="
echo "Go version: $(go version)"
echo ""

echo "--- [1/5] Build all packages ---"
go build ./...
echo "PASS: build"

echo "--- [2/5] Vet ---"
go vet ./...
echo "PASS: vet"

echo "--- [3/5] Unit tests (race detector on critical packages) ---"
go test ./... -count=1 -timeout=300s
echo "PASS: unit tests"
go test -race ./pkg/security/... ./pkg/persistence/... ./pkg/loadtest/... -count=1 -timeout=120s
echo "PASS: race detector"

echo "--- [4/5] Integration tests ---"
go test ./tests/integration/... -count=1 -timeout=120s
echo "PASS: integration tests"

echo "--- [5/5] Benchmark smoke run ---"
go test ./benchmarks/production/... -bench=. -benchmem -benchtime=1x -timeout=120s
echo "PASS: benchmarks"

echo ""
echo "=== RC Validation COMPLETE ==="
echo "All gates passed. Ready for release tagging."
