#!/bin/bash
set -euo pipefail

echo "=== Verifying clean baseline ==="

echo "1. Running cargo check..."
cargo check -p services

echo "2. Running cargo test..."
cargo test -p services

echo "3. Checking for compilation errors..."
ERRORS=$(cargo check -p services 2>&1 | grep "^error" | wc -l)
if [[ "$ERRORS" -ne 0 ]]; then
    echo "❌ Found $ERRORS compilation errors"
    exit 1
fi

echo "✅ Clean baseline verified!"
echo "   - Zero compilation errors"
echo "   - All tests passing"
