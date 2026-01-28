#!/usr/bin/env bash
# scripts/quality-check.sh - Run all quality checks
set -e

echo "=== VCDeploy Quality Checks ==="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

FAILED=0

# Function to run a check
run_check() {
    local name="$1"
    local cmd="$2"
    echo -n "Running $name... "
    if eval "$cmd" > /tmp/quality-check-output.txt 2>&1; then
        echo -e "${GREEN}PASSED${NC}"
    else
        echo -e "${RED}FAILED${NC}"
        cat /tmp/quality-check-output.txt
        FAILED=1
    fi
}

# Check for required tools
check_tool() {
    local tool="$1"
    if ! command -v "$tool" &> /dev/null; then
        echo -e "${YELLOW}Warning: $tool not installed${NC}"
        return 1
    fi
    return 0
}

echo "Checking required tools..."
echo ""

# 1. Go fmt check
echo "--- Format Checks ---"
run_check "gofmt" "test -z \"\$(gofmt -l . 2>/dev/null | grep -v vendor)\""

# 2. Go imports check (if available)
if check_tool goimports; then
    run_check "goimports" "test -z \"\$(goimports -l . 2>/dev/null | grep -v vendor)\""
fi

echo ""
echo "--- Static Analysis ---"

# 3. Go vet
run_check "go vet" "go vet ./..."

# 4. Golangci-lint (if available)
if check_tool golangci-lint; then
    run_check "golangci-lint" "golangci-lint run ./..."
fi

echo ""
echo "--- Security Checks ---"

# 5. Go vulnerabilities (if available)
if check_tool govulncheck; then
    run_check "govulncheck" "govulncheck ./..."
fi

# 6. Gosec (if available)
if check_tool gosec; then
    run_check "gosec" "gosec -exclude=G104,G115,G204,G301,G302,G304,G306 -quiet ./..."
fi

echo ""
echo "--- Build Check ---"

# 7. Build check
run_check "go build" "go build ./..."

echo ""
echo "=== Summary ==="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All checks passed!${NC}"
    exit 0
else
    echo -e "${RED}Some checks failed!${NC}"
    exit 1
fi
