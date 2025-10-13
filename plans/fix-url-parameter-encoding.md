# URL Parameter Encoding Fix

## Problem

When MJML templates containing `mj-button` elements with URLs that had **already-escaped** ampersands (e.g., `&amp;`) were converted to MJML, the backend was **double-escaping** them, resulting in broken URLs.

### Example of the Bug

**Input URL (already escaped):**
```
https://example.com?a=1&amp;b=2
```

**Wrong Output (double-escaped):**
```xml
<mj-button href="https://example.com?a=1&amp;amp;b=2">
```

**Browser receives:**
```
https://example.com?a=1&amp;b=2  ❌ BROKEN!
```

## Root Cause

The `escapeAttributeValue()` function in `pkg/notifuse_mjml/converter.go` was blindly escaping ALL ampersands using:

```go
value = strings.ReplaceAll(value, "&", "&amp;")
```

This meant that `&amp;` would become `&amp;amp;`, causing double-escaping.

## Solution

### Best Practice for URLs in MJML/HTML

✅ **URLs in XML/HTML attributes MUST have ampersands escaped as `&amp;`**

```xml
<!-- ✅ CORRECT -->
<mj-button href="https://example.com?a=1&amp;b=2">Click</mj-button>

<!-- ❌ WRONG (Invalid XML) -->
<mj-button href="https://example.com?a=1&b=2">Click</mj-button>
```

**Why?**
- MJML is XML, and XML requires `&` to be escaped in attributes
- The MJML compiler converts to HTML (which also requires `&amp;`)
- The browser automatically decodes `&amp;` → `&` when making HTTP requests

### Implementation

Created a new function `escapeUnescapedAmpersands()` that:

1. **Finds all XML/HTML entities** (e.g., `&amp;`, `&lt;`, `&#123;`, `&#xAB;`)
2. **Preserves them as-is** (they're already properly escaped)
3. **Only escapes bare `&` characters** that are not part of an entity

```go
// escapeUnescapedAmpersands escapes only unescaped ampersands in a string
// It skips ampersands that are already part of XML entities like &amp;, &lt;, &#123;, etc.
func escapeUnescapedAmpersands(value string) string {
	// Pattern matches XML/HTML entities: &amp; &lt; &gt; &quot; &apos; &#123; &#xAB; etc.
	entityPattern := regexp.MustCompile(`&(amp|lt|gt|quot|apos|#\d+|#x[0-9a-fA-F]+);`)
	
	var result strings.Builder
	lastEnd := 0
	
	// Find all entities and preserve them
	matches := entityPattern.FindAllStringIndex(value, -1)
	
	for _, match := range matches {
		start, end := match[0], match[1]
		
		// Process the part before this entity
		beforeEntity := value[lastEnd:start]
		result.WriteString(strings.ReplaceAll(beforeEntity, "&", "&amp;"))
		
		// Add the entity as-is (it's already escaped)
		result.WriteString(value[start:end])
		
		lastEnd = end
	}
	
	// Process the remaining part after the last entity
	remaining := value[lastEnd:]
	result.WriteString(strings.ReplaceAll(remaining, "&", "&amp;"))
	
	return result.String()
}
```

## Test Coverage

Added comprehensive tests in `pkg/notifuse_mjml/converter_url_encoding_test.go`:

### Test Functions

1. **`TestEscapeUnescapedAmpersands`** - 11 test cases covering:
   - Basic URLs with unescaped ampersands
   - URLs with already escaped ampersands
   - Mixed escaped and unescaped ampersands
   - Preservation of other XML entities (`&lt;`, `&gt;`, `&quot;`, etc.)
   - Numeric entities (`&#169;`, `&#xA9;`)
   - Plain text with ampersands

2. **`TestEscapeAttributeValue`** - 8 test cases covering:
   - URL escaping scenarios
   - Quote escaping
   - Angle bracket escaping
   - Complex mixed scenarios

3. **`TestMJMLButtonURLEncoding`** - 4 test cases covering:
   - Buttons with unescaped ampersands
   - Buttons with already escaped ampersands
   - Buttons with mixed escaping
   - Buttons with UTM parameters

### Test Results

```bash
$ go test ./pkg/notifuse_mjml/... -v
# All 100+ tests PASS ✅
```

## Verification

### Before Fix

```
Input:  "https://example.com?a=1&amp;b=2"
Output: <mj-button href="https://example.com?a=1&amp;amp;b=2"> ❌
```

### After Fix

```
Input:  "https://example.com?a=1&b=2"
Output: <mj-button href="https://example.com?a=1&amp;b=2"> ✅

Input:  "https://example.com?a=1&amp;b=2"  
Output: <mj-button href="https://example.com?a=1&amp;b=2"> ✅
```

Both inputs now produce the correct, identical output!

## Files Changed

1. **`pkg/notifuse_mjml/converter.go`**
   - Modified `escapeAttributeValue()` to use new function
   - Added `escapeUnescapedAmpersands()` function

2. **`pkg/notifuse_mjml/converter_url_encoding_test.go`** (new file)
   - Added 23 comprehensive test cases

## Alignment with Frontend

The fix aligns the backend behavior with the frontend's existing implementation in `console/src/components/mjml-converter/mjml-to-json-browser.ts` (line 52):

```javascript
const fixed = attrValue.replace(/&(?!(amp|lt|gt|quot|apos|#\d+|#x[0-9a-fA-F]+);)/g, '&amp;')
```

The frontend uses a negative lookahead regex, which Go doesn't support, so we implemented an equivalent algorithm using `FindAllStringIndex`.

## Impact

- ✅ No breaking changes
- ✅ All existing tests pass
- ✅ Fixes double-escaping bug
- ✅ Maintains correct XML/HTML escaping
- ✅ Compatible with all URL types (query parameters, fragments, etc.)
- ✅ Preserves all existing XML entities
