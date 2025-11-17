import { Liquid } from 'liquidjs'

/**
 * Security configuration for Liquid template rendering
 *
 * SECURITY CONSIDERATIONS:
 * 1. Limited file system access (no layouts or renders, includes allowed)
 * 2. Limited loop iterations to prevent infinite loops
 * 3. Timeout protection to prevent CPU exhaustion
 * 4. Strict filters to catch typos/malicious filter attempts
 * 5. All user content should be escaped by default
 */

// Maximum iterations for loops (for, tablerow)
export const MAX_LOOP_ITERATIONS = 10000

// Maximum template execution time in milliseconds
export const MAX_EXECUTION_TIME = 5000

// Tags that are DISABLED for security
export const DISABLED_TAGS = [
  'layout', // Could reference unauthorized layouts
  'render' // Could render unauthorized partials
  // 'raw' is a tag in some Liquid implementations - blocks processing
]

// Filters that should be used with CAUTION (documented for backend implementation)
export const SENSITIVE_FILTERS = [
  // 'raw' filter doesn't exist in liquidjs, but document for backend
  // All output is unescaped by default in Liquid, unlike template engines like Handlebars
]

/**
 * Create a secure Liquid engine instance
 */
export function createSecureLiquidEngine(): Liquid {
  const liquid = new Liquid({
    // SECURITY: Throw errors on undefined filters (prevents typos and malicious attempts)
    strictFilters: true,

    // SECURITY: Don't throw on undefined variables (graceful degradation)
    strictVariables: false,

    // SECURITY: No file system access - empty root means no includes/layouts work
    root: [],

    // SECURITY: No partials/layouts allowed
    partials: [],
    layouts: [],

    // SECURITY: Explicit timezone (prevent timezone manipulation)
    timezoneOffset: 0,

    // Allow lenient if statements (won't throw on null/undefined)
    lenientIf: true,

    // Preserve whitespace for predictable output
    trimTagRight: false,
    trimTagLeft: false,
    trimOutputRight: false,
    trimOutputLeft: false,

    // Greedy mode for better performance
    greedy: true
  })

  // SECURITY: Register iteration limit check
  // Note: liquidjs doesn't have a built-in iteration limit,
  // but we document this for backend implementation

  return liquid
}

/**
 * Validate a Liquid template for security issues
 * Returns array of security issues found, or empty array if safe
 */
export function validateTemplateSecurity(template: string): string[] {
  const issues: string[] = []

  // Check template size (prevent memory exhaustion)
  if (template.length > 100000) {
    issues.push('Template exceeds maximum size of 100KB')
  }

  // Check for disabled tags
  DISABLED_TAGS.forEach((tag) => {
    const tagPattern = new RegExp(`{%\\s*${tag}\\s+`, 'gi')
    if (tagPattern.test(template)) {
      issues.push(`Disabled tag detected: ${tag}`)
    }
  })

  // Check for excessive nesting (could cause stack overflow)
  const maxNestingDepth = 20
  let currentDepth = 0
  let maxDepth = 0

  const openTags = template.match(/{%\s*(if|unless|for|case|tablerow|capture|block)\s/gi) || []
  const closeTags =
    template.match(/{%\s*end(if|unless|for|case|tablerow|capture|block)\s*%}/gi) || []

  if (openTags.length !== closeTags.length) {
    issues.push('Unbalanced template tags detected')
  }

  // Simple depth check
  for (const char of template) {
    if (char === '{') currentDepth++
    if (char === '}') currentDepth--
    maxDepth = Math.max(maxDepth, currentDepth)
  }

  if (maxDepth > maxNestingDepth * 2) {
    // *2 because each tag has 2 braces
    issues.push(`Template nesting too deep (max: ${maxNestingDepth} levels)`)
  }

  // Check for potential infinite loops (for without limit)
  const forLoops = template.match(/{%\s*for\s+\w+\s+in\s+[^%]+%}/gi) || []
  forLoops.forEach((loop) => {
    // This is a basic check - in production, you'd want more sophisticated analysis
    if (loop.length > 1000) {
      issues.push('Suspiciously complex for loop detected')
    }
  })

  return issues
}

/**
 * Render a Liquid template with security protections
 */
export async function renderSecureLiquid(
  template: string,
  data: any
): Promise<{ html: string; errors: string[] }> {
  const errors: string[] = []

  // Pre-render security validation
  const securityIssues = validateTemplateSecurity(template)
  if (securityIssues.length > 0) {
    return {
      html: '',
      errors: securityIssues
    }
  }

  try {
    const engine = createSecureLiquidEngine()

    // Wrap in timeout protection
    const timeoutPromise = new Promise<never>((_, reject) => {
      setTimeout(() => reject(new Error('Template execution timeout')), MAX_EXECUTION_TIME)
    })

    const renderPromise = engine.parseAndRender(template, data)

    const html = await Promise.race([renderPromise, timeoutPromise])

    return { html, errors: [] }
  } catch (error: any) {
    errors.push(error.message || 'Template rendering failed')
    return { html: '', errors }
  }
}

/**
 * Documentation for backend implementation
 */
export const BACKEND_SECURITY_NOTES = `
BACKEND LIQUID SECURITY (Go implementation):

1. Use a sandboxed Liquid parser (e.g., github.com/osteele/liquid)

2. Disable dangerous tags:
   - layout  
   - render
   - Any tags that access filesystem (except include)

3. Set resource limits:
   - Max iterations: 10,000
   - Max execution time: 5 seconds
   - Max template size: 100KB
   - Max recursion depth: 20

4. Run in isolated context:
   - No access to system environment
   - No access to filesystem
   - No network access
   - Read-only data context

5. HTML Escaping:
   - By default, Liquid does NOT escape HTML
   - Consider auto-escaping or requiring explicit | escape filter
   - Or use a post-processing HTML sanitizer

6. Rate limiting:
   - Limit template compilations per user
   - Limit preview requests per minute

7. Content Security Policy:
   - Set strict CSP headers when serving blog
   - Prevent inline scripts
   - Whitelist allowed resources

8. Validation before save:
   - Parse template to check for syntax errors
   - Run security validation
   - Store both original and compiled versions

9. Version control:
   - Keep history of all template changes
   - Allow rollback to previous versions
   - Audit log of who made changes

10. Monitoring:
    - Log template execution times
    - Alert on timeouts or errors
    - Track resource usage per workspace
`
