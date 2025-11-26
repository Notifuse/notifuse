#!/bin/bash
set -e

# Extract the file path from hook input
FILE=$(jq -r '.files[0].path // empty' 2>/dev/null || echo "")

# Only process Go files
if [[ ! "$FILE" =~ \.go$ || ! -f "$FILE" ]]; then
  exit 0
fi

echo "Formatting $FILE with gofmt..."
gofmt -w "$FILE"

echo "Running go vet..."
PACKAGE_DIR=$(dirname "$FILE")
cd "$PACKAGE_DIR"
go vet ./...

exit 0
