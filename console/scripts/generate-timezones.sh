#!/bin/bash
#
# Generate TypeScript timezone list from backend Go timezone file
#
# This script extracts timezone identifiers from the backend's timezone list
# and generates a TypeScript file for the frontend.
#
# Usage:
#   ./console/scripts/generate-timezones.sh
#
# Prerequisites:
#   - Backend timezones must be up to date (run: cd internal/domain && go run generate_timezones.go)
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BACKEND_TZ_FILE="$REPO_ROOT/internal/domain/timezones.go"
OUTPUT_FILE="$REPO_ROOT/console/src/lib/timezones.ts"

if [ ! -f "$BACKEND_TZ_FILE" ]; then
  echo "❌ Error: Backend timezone file not found: $BACKEND_TZ_FILE"
  exit 1
fi

echo "🔄 Generating frontend timezone list..."
echo "   Source: $BACKEND_TZ_FILE"
echo "   Output: $OUTPUT_FILE"
echo ""

# Generate the TypeScript file
cat > "$OUTPUT_FILE" << 'EOF'
/**
 * Valid IANA Timezone Identifiers
 * 
 * This list is automatically generated from the backend's timezone database.
 * It includes both canonical timezones and their aliases (Link zones).
 * 
 * Source: internal/domain/timezones.go
 * Generated from: Go's embedded IANA timezone database
 * 
 * To regenerate this list:
 * 1. Update backend: cd internal/domain && go run generate_timezones.go
 * 2. Update frontend: ./console/scripts/generate-timezones.sh
 */

/**
 * Array of all valid IANA timezone identifiers accepted by the backend
 */
export const VALID_TIMEZONES: readonly string[] = [
EOF

# Extract timezone strings from Go file
grep '^\s*"' "$BACKEND_TZ_FILE" | \
  sed 's/^[[:space:]]*/  /' >> "$OUTPUT_FILE"

cat >> "$OUTPUT_FILE" << 'EOF'
] as const

/**
 * Type representing any valid timezone identifier
 */
export type TimezoneIdentifier = typeof VALID_TIMEZONES[number]

/**
 * Form options for Ant Design Select component
 */
export const TIMEZONE_OPTIONS = VALID_TIMEZONES.map(tz => ({
  value: tz,
  label: tz,
}))

/**
 * Checks if a timezone string is valid according to the backend
 * 
 * @param timezone - The timezone identifier to validate
 * @returns true if the timezone is valid
 */
export function isValidTimezone(timezone: string): timezone is TimezoneIdentifier {
  return VALID_TIMEZONES.includes(timezone as TimezoneIdentifier)
}

/**
 * Total number of valid timezones
 */
export const TIMEZONE_COUNT = VALID_TIMEZONES.length
EOF

# Count timezones
TZ_COUNT=$(grep -c '^\s*"' "$BACKEND_TZ_FILE")

echo "✅ Generated $OUTPUT_FILE"
echo "   Total timezones: $TZ_COUNT"
echo ""
echo "🎉 Frontend timezone list updated successfully!"
