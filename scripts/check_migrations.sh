#!/usr/bin/env bash
set -euo pipefail

# Check if migrations are modified or deleted compared to target branch (default origin/main)
TARGET_BRANCH="${1:-origin/main}"

echo "Checking migration script immutability against ${TARGET_BRANCH}..."

# Ensure target branch exists locally
if ! git rev-parse --verify "${TARGET_BRANCH}" >/dev/null 2>&1; then
    echo "Notice: Target branch ${TARGET_BRANCH} not found. Skipping git diff check (initial branch or first run)."
    exit 0
fi

# Look for Modified (M) or Deleted (D) files in migrations/
MODIFIED_OR_DELETED=$(git diff --name-status "${TARGET_BRANCH}...HEAD" -- migrations/ | grep -E '^[MD]' || true)

if [ -n "${MODIFIED_OR_DELETED}" ]; then
    echo "ERROR: Historical migration scripts must remain immutable!"
    echo "Modified or deleted migration files detected:"
    echo "${MODIFIED_OR_DELETED}"
    echo "Please create a new migration version instead of altering historical migration files."
    exit 1
fi

echo "Migration immutability check passed."
