#!/bin/bash

# run-eval.sh - Convenience script to run evaluations

set -e

# Load environment variables
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Default values
TEST_CASE="${1:-example_tasks}"
RUN_ID="${2:-run_$(date +%s)}"
PARALLEL="${3:-3}"

echo "Running evaluation:"
echo "  Test case: $TEST_CASE"
echo "  Run ID: $RUN_ID"
echo "  Parallelism: $PARALLEL"
echo ""

# Build if needed
if [ ! -f bin/mix-eval-go ]; then
    echo "Building mix-eval-go..."
    mkdir -p bin
    go build -o bin/mix-eval-go cmd/main.go
fi

# Run evaluation
./bin/mix-eval-go \
    --test-case "$TEST_CASE" \
    --run-id "$RUN_ID" \
    --parallel "$PARALLEL"
