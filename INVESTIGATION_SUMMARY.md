# Timezone Name Mismatch Investigation & Fix

## Issue Report

**Original Problem:**
- Frontend sending `"timezone":"Asia/Calcutta"` to `/api/workspaces.create`
- Backend rejecting with validation error
- Manually changing to `"timezone":"Asia/Kolkata"` in curl works fine

## Investigation Results

### ✅ Issue Confirmed

The problem is a mismatch between:
1. **Browser API** → Returns deprecated IANA timezone aliases (e.g., "Asia/Calcutta")
2. **Backend validation** → Only accepts canonical IANA timezone names (e.g., "Asia/Kolkata")

### Test Results

```javascript
// Node.js Intl API behavior (same as browser):
Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Calcutta' }).resolvedOptions().timeZone
// Returns: "Asia/Calcutta"

Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Kolkata' }).resolvedOptions().timeZone
// Returns: "Asia/Calcutta" (normalized to deprecated alias!)
```

### Who is Using Non-ISO Names?

**Answer: The Browser/Frontend** ❌

The backend is correct and follows the official IANA timezone database. The browser's `Intl` API returns deprecated timezone aliases that are not in the canonical list.

## IANA Timezone Database

The IANA timezone database has two types of timezone entries:

1. **Canonical zones** - Official current names (e.g., `Asia/Kolkata`)
2. **Links/Aliases** - Deprecated names that point to canonical zones (e.g., `Asia/Calcutta` → `Asia/Kolkata`)

**Backend Implementation:**
- File: `internal/domain/timezones.go`
- Contains: 429 canonical IANA timezone names
- Validates with: `IsValidTimezone()` function
- Uses: Official IANA timezone database (canonical names only)

**Frontend JSON File:**
- File: `console/src/lib/countries_timezones.json`
- Contains: Canonical names only (e.g., India → `"Asia/Kolkata"`)
- Source: Properly curated with canonical IANA names

## Root Cause Analysis

### Problem Locations

The issue occurred in **2 places** where the frontend auto-detects browser timezone:

1. **CreateWorkspacePage.tsx** (line 54)
   ```typescript
   // ❌ BEFORE - Returns deprecated aliases
   const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
   ```

2. **AnalyticsPage.tsx** (line 22)
   ```typescript
   // ❌ BEFORE - Returns deprecated aliases
   const browserTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone
   ```

### Why Manual Selection Works

When users manually select a timezone from the dropdown in Workspace Settings:
- The dropdown uses `TimezonesFormOptions` from `countries_timezones.json`
- This JSON file already contains canonical names
- Therefore manual selection always sends correct timezone names ✅

## Solution Implemented

### 1. Created Timezone Normalizer Utility

**File:** `console/src/lib/timezoneNormalizer.ts`

- Maps 100+ deprecated timezone aliases to canonical names
- Provides `getBrowserTimezone()` function
- Includes comprehensive coverage:
  - Deprecated city names (Calcutta → Kolkata, Kiev → Kyiv)
  - Legacy zone abbreviations (US/*, Canada/*, Mexico/*)
  - Country-based zones (Japan, Egypt, Turkey)
  - UTC variations (GMT, Etc/UTC, etc.)

**API:**
```typescript
import { getBrowserTimezone, normalizeTimezone, isDeprecatedTimezone } from '@/lib/timezoneNormalizer'

// Get browser timezone (automatically normalized)
const timezone = getBrowserTimezone()
// Returns: "Asia/Kolkata" (even if browser returns "Asia/Calcutta")

// Manually normalize
const normalized = normalizeTimezone("Asia/Calcutta")
// Returns: "Asia/Kolkata"

// Check if deprecated
const isDeprecated = isDeprecatedTimezone("Asia/Calcutta")
// Returns: true
```

### 2. Updated Frontend Code

**CreateWorkspacePage.tsx:**
```typescript
// ✅ AFTER - Returns canonical name
import { getBrowserTimezone } from '../lib/timezoneNormalizer'
const timezone = getBrowserTimezone()
```

**AnalyticsPage.tsx:**
```typescript
// ✅ AFTER - Returns canonical name
import { getBrowserTimezone } from '../lib/timezoneNormalizer'
const browserTimezone = getBrowserTimezone()
```

### 3. Added Test Coverage

**File:** `console/src/lib/timezoneNormalizer.test.ts`

```bash
✓ src/lib/timezoneNormalizer.test.ts (13 tests) 14ms
  ✓ normalizeTimezone (8 tests)
  ✓ isDeprecatedTimezone (3 tests)
  ✓ getBrowserTimezone (2 tests)

Test Files  1 passed (1)
Tests       13 passed (13)
```

## Affected Users

### Before Fix ❌

Users with browsers returning deprecated timezone names would get errors:
- 🇮🇳 India users → `Asia/Calcutta` → ❌ API error
- 🇺🇦 Ukraine users → `Europe/Kiev` → ❌ API error
- 🇺🇸 US users with old browsers → `US/Eastern` → ❌ API error
- 🇦🇷 Argentina users → `America/Buenos_Aires` → ❌ API error

### After Fix ✅

All users get seamless experience:
- 🇮🇳 India users → `Asia/Calcutta` → normalized to → `Asia/Kolkata` ✅
- 🇺🇦 Ukraine users → `Europe/Kiev` → normalized to → `Europe/Kyiv` ✅
- 🇺🇸 US users → `US/Eastern` → normalized to → `America/New_York` ✅
- 🇦🇷 Argentina users → `America/Buenos_Aires` → normalized to → `America/Argentina/Buenos_Aires` ✅

## Common Timezone Aliases Handled

| Deprecated Name | Canonical Name |
|-----------------|----------------|
| `Asia/Calcutta` | `Asia/Kolkata` |
| `Europe/Kiev` | `Europe/Kyiv` |
| `US/Eastern` | `America/New_York` |
| `US/Pacific` | `America/Los_Angeles` |
| `US/Central` | `America/Chicago` |
| `Canada/Eastern` | `America/Toronto` |
| `America/Buenos_Aires` | `America/Argentina/Buenos_Aires` |
| `Japan` | `Asia/Tokyo` |
| `Egypt` | `Africa/Cairo` |
| `Turkey` | `Europe/Istanbul` |

## Verification Steps

### 1. TypeScript Compilation
```bash
cd console && npx tsc --noEmit
# ✅ No errors
```

### 2. Unit Tests
```bash
cd console && npm test -- timezoneNormalizer.test.ts --run
# ✅ 13 tests passed
```

### 3. Manual Testing
Create a workspace from a browser/location that returns "Asia/Calcutta":
- Frontend detects: `Asia/Calcutta`
- Normalizer converts: `Asia/Kolkata`
- Backend receives: `Asia/Kolkata`
- Validation passes: ✅

## Files Changed

1. ✅ `console/src/lib/timezoneNormalizer.ts` - New utility (147 lines)
2. ✅ `console/src/lib/timezoneNormalizer.test.ts` - Test file (78 lines)
3. ✅ `console/src/pages/CreateWorkspacePage.tsx` - Updated import + usage
4. ✅ `console/src/pages/AnalyticsPage.tsx` - Updated import + usage
5. ✅ `TIMEZONE_FIX.md` - Documentation
6. ✅ `INVESTIGATION_SUMMARY.md` - This file

## References

- [IANA Time Zone Database](https://www.iana.org/time-zones)
- [Wikipedia: List of tz database time zones](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones)
- [MDN: Intl.DateTimeFormat](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Intl/DateTimeFormat)

## Conclusion

✅ **Issue Verified:** Frontend uses deprecated timezone aliases from browser API  
✅ **Backend Correct:** Uses official IANA canonical names  
✅ **Solution Implemented:** Timezone normalizer utility  
✅ **Tests Passing:** 13 comprehensive tests  
✅ **No Breaking Changes:** Backward compatible  
✅ **Production Ready:** Safe to deploy
