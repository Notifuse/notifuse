# Timezone Name Mismatch Fix

## Problem

The frontend was sending deprecated IANA timezone aliases (e.g., `Asia/Calcutta`) to the backend, which caused validation errors because the backend only accepts canonical IANA timezone names (e.g., `Asia/Kolkata`).

### Root Cause

The issue occurred when using the browser's `Intl.DateTimeFormat().resolvedOptions().timeZone` API to detect the user's timezone. This API can return **legacy timezone aliases** that are no longer the canonical names in the IANA timezone database.

#### Examples of Browser Behavior:
```javascript
Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Calcutta' }).resolvedOptions().timeZone
// Returns: "Asia/Calcutta" (deprecated alias)

Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Kolkata' }).resolvedOptions().timeZone
// Returns: "Asia/Calcutta" (normalizes to deprecated alias!)
```

### Who is Correct?

- ✅ **Backend**: Uses official IANA timezone database with canonical names
- ❌ **Frontend**: Browser APIs return deprecated timezone aliases

### Historical Context

Many timezone names have changed over time:
- **Asia/Calcutta** → **Asia/Kolkata** (city renamed in 2001)
- **Europe/Kiev** → **Europe/Kyiv** (updated Ukrainian transliteration)
- **US/Eastern** → **America/New_York** (deprecated zone abbreviation)

The IANA timezone database maintains these as "links" (aliases) for backward compatibility, but they are not canonical names.

## Solution

Created a timezone normalization utility (`console/src/lib/timezoneNormalizer.ts`) that:

1. Maps deprecated timezone aliases to their canonical IANA names
2. Provides a `getBrowserTimezone()` function that automatically normalizes the browser's timezone
3. Includes comprehensive mappings for:
   - Deprecated city names (Calcutta, Kiev, etc.)
   - Legacy zone abbreviations (US/*, Canada/*, etc.)
   - Country-based zones (Japan, Egypt, Turkey, etc.)
   - UTC variations (GMT, Etc/UTC, etc.)

### Implementation

The fix was applied in two locations where the frontend detects the user's timezone:

1. **CreateWorkspacePage.tsx** (line 54)
   - Changed from: `Intl.DateTimeFormat().resolvedOptions().timeZone`
   - Changed to: `getBrowserTimezone()`

2. **AnalyticsPage.tsx** (line 22)
   - Changed from: `Intl.DateTimeFormat().resolvedOptions().timeZone`
   - Changed to: `getBrowserTimezone()`

### API Reference

```typescript
import { getBrowserTimezone, normalizeTimezone, isDeprecatedTimezone } from '@/lib/timezoneNormalizer'

// Get browser timezone (automatically normalized)
const timezone = getBrowserTimezone()
// Returns: "Asia/Kolkata" (even if browser returns "Asia/Calcutta")

// Manually normalize a timezone
const normalized = normalizeTimezone("Asia/Calcutta")
// Returns: "Asia/Kolkata"

// Check if a timezone is deprecated
const isDeprecated = isDeprecatedTimezone("Asia/Calcutta")
// Returns: true
```

## Testing

Added comprehensive test coverage (`console/src/lib/timezoneNormalizer.test.ts`):
- ✅ 13 tests covering all major timezone aliases
- ✅ Validation that canonical names remain unchanged
- ✅ Verification that unknown timezones are handled gracefully

Run tests:
```bash
cd console && npm test -- timezoneNormalizer.test.ts
```

## Impact

### Before Fix
- Users in India (Asia/Calcutta) → ❌ Workspace creation failed
- Users in Ukraine (Europe/Kiev) → ❌ Workspace creation failed
- Users with legacy browser timezones → ❌ API errors

### After Fix
- All users → ✅ Timezone automatically normalized to canonical name
- Backend validation → ✅ Always receives valid IANA timezone
- No user-facing errors → ✅ Seamless experience

## References

- [IANA Time Zone Database](https://www.iana.org/time-zones)
- [Wikipedia: List of tz database time zones](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones)
- [MDN: Intl.DateTimeFormat](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Intl/DateTimeFormat)

## Backend Validation

The backend validates timezones using the canonical IANA list in:
- `internal/domain/timezones.go` - Contains all 429 canonical IANA timezones
- `internal/domain/workspace.go` - Validates timezone using `IsValidTimezone()`

This list is based on the official IANA timezone database and uses only canonical names.
