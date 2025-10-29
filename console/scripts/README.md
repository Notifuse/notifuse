# Console Scripts

This directory contains utility scripts for the Notifuse console frontend.

## Scripts

### `generate-timezones.sh`

Generates the frontend timezone list from the backend's Go timezone database.

**Purpose:**
- Synchronizes frontend timezone list with backend
- Extracts all 594 timezone identifiers from `internal/domain/timezones.go`
- Generates TypeScript file with type-safe timezone constants

**Usage:**
```bash
./console/scripts/generate-timezones.sh
```

**When to run:**
- After updating backend timezone list
- After upgrading Go version (which may add new timezones)
- When backend runs: `cd internal/domain && go run generate_timezones.go`

**Output:**
- Generates: `console/src/lib/timezones.ts`
- Includes: `VALID_TIMEZONES`, `TIMEZONE_OPTIONS`, `isValidTimezone()`, `TimezoneIdentifier` type

**Workflow:**
1. Update backend: `cd internal/domain && go run generate_timezones.go`
2. Update frontend: `./console/scripts/generate-timezones.sh`
3. Test: `npm test` (in console directory)
4. Commit both backend and frontend changes together

## Adding New Scripts

When adding new scripts:
1. Make them executable: `chmod +x script-name.sh`
2. Add documentation here
3. Use clear naming conventions
4. Include error handling
5. Add usage examples
