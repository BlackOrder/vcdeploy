#!/usr/bin/env bash
# scripts/test-all.sh - Run all tests with optional parallel control
set -e

echo "=== VCDeploy Test Runner ==="
echo ""

# Parse arguments
PARALLEL=true
COVERAGE=false
E2E=false
CLI=false
UI=false
UNIT=true

while [[ $# -gt 0 ]]; do
    case $1 in
        --no-parallel)
            PARALLEL=false
            export TEST_NO_PARALLEL=1
            shift
            ;;
        --coverage)
            COVERAGE=true
            shift
            ;;
        --e2e)
            E2E=true
            UNIT=false
            shift
            ;;
        --cli)
            CLI=true
            UNIT=false
            shift
            ;;
        --ui)
            UI=true
            UNIT=false
            shift
            ;;
        --all)
            E2E=true
            CLI=true
            UI=true
            UNIT=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--no-parallel] [--coverage] [--e2e] [--cli] [--ui] [--all]"
            exit 1
            ;;
    esac
done

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

FAILED=0

if [ "$PARALLEL" = false ]; then
    echo -e "${YELLOW}Running tests in single-worker mode${NC}"
    echo ""
fi

# Unit tests
if [ "$UNIT" = true ]; then
    echo "--- Running Unit Tests ---"
    TEST_FLAGS="-v -race"
    if [ "$COVERAGE" = true ]; then
        TEST_FLAGS="$TEST_FLAGS -coverprofile=coverage.out"
    fi
    if go test $TEST_FLAGS -short -timeout 10m ./...; then
        echo -e "${GREEN}Unit tests passed${NC}"
    else
        echo -e "${RED}Unit tests failed${NC}"
        FAILED=1
    fi
    echo ""
fi

# E2E API tests
if [ "$E2E" = true ]; then
    echo "--- Running E2E API Tests ---"
    if go test -v -tags=e2e -timeout 10m ./tests/e2e/...; then
        echo -e "${GREEN}E2E tests passed${NC}"
    else
        echo -e "${RED}E2E tests failed${NC}"
        FAILED=1
    fi
    echo ""
fi

# CLI tests
if [ "$CLI" = true ]; then
    echo "--- Running CLI Tests ---"
    if go test -v -tags=cli -timeout 10m ./tests/cli/...; then
        echo -e "${GREEN}CLI tests passed${NC}"
    else
        echo -e "${RED}CLI tests failed${NC}"
        FAILED=1
    fi
    echo ""
fi

# UI tests (Playwright)
if [ "$UI" = true ]; then
    echo "--- Running UI Tests (Playwright) ---"
    cd tests/playwright
    
    PLAYWRIGHT_FLAGS=""
    if [ "$PARALLEL" = false ]; then
        PLAYWRIGHT_FLAGS="--workers=1"
    fi
    
    if npx playwright test $PLAYWRIGHT_FLAGS; then
        echo -e "${GREEN}UI tests passed${NC}"
    else
        echo -e "${RED}UI tests failed${NC}"
        FAILED=1
    fi
    cd ../..
    echo ""
fi

# Summary
echo "=== Test Summary ==="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
fi
