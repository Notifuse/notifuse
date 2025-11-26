#!/bin/bash
set -e

# Extract the file path from hook input
FILE=$(jq -r '.files[0].path // empty' 2>/dev/null || echo "")

# Only process TypeScript/React files in console directory
if [[ ! "$FILE" =~ console/.*\.(ts|tsx)$ || ! -f "$FILE" ]]; then
  exit 0
fi

echo "Linting $FILE with ESLint..."

# Run ESLint from console directory on the specific file
cd console
npx eslint "../$FILE" --max-warnings 0

exit 0
