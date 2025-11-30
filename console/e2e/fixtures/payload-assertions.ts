/**
 * Payload assertion utilities for verifying API request payloads in e2e tests.
 * These helpers verify that form data is correctly sent to the API.
 */

import { expect } from '@playwright/test'
import { RequestCaptureStore, CapturedRequest, ApiPattern } from './request-capture'

/**
 * Deep partial matching - checks that all properties in expected exist in actual
 * with matching values. Allows actual to have additional properties.
 */
export function deepPartialMatch(
  actual: unknown,
  expected: unknown,
  path: string = ''
): { matches: boolean; errors: string[] } {
  const errors: string[] = []

  if (expected === undefined) {
    return { matches: true, errors: [] }
  }

  if (expected === null) {
    if (actual !== null) {
      errors.push(`${path}: expected null but got ${JSON.stringify(actual)}`)
    }
    return { matches: errors.length === 0, errors }
  }

  if (typeof expected !== typeof actual) {
    errors.push(
      `${path}: type mismatch - expected ${typeof expected} but got ${typeof actual} (${JSON.stringify(actual)})`
    )
    return { matches: false, errors }
  }

  if (Array.isArray(expected)) {
    if (!Array.isArray(actual)) {
      errors.push(`${path}: expected array but got ${typeof actual}`)
      return { matches: false, errors }
    }

    // For arrays, check that all expected elements exist in actual
    for (let i = 0; i < expected.length; i++) {
      const result = deepPartialMatch(actual[i], expected[i], `${path}[${i}]`)
      errors.push(...result.errors)
    }
    return { matches: errors.length === 0, errors }
  }

  if (typeof expected === 'object') {
    if (typeof actual !== 'object' || actual === null) {
      errors.push(`${path}: expected object but got ${typeof actual}`)
      return { matches: false, errors }
    }

    const actualObj = actual as Record<string, unknown>
    const expectedObj = expected as Record<string, unknown>

    for (const key of Object.keys(expectedObj)) {
      const newPath = path ? `${path}.${key}` : key
      if (!(key in actualObj)) {
        errors.push(`${newPath}: missing in actual payload`)
      } else {
        const result = deepPartialMatch(actualObj[key], expectedObj[key], newPath)
        errors.push(...result.errors)
      }
    }
    return { matches: errors.length === 0, errors }
  }

  // Primitive comparison
  if (actual !== expected) {
    errors.push(`${path}: expected ${JSON.stringify(expected)} but got ${JSON.stringify(actual)}`)
    return { matches: false, errors }
  }

  return { matches: true, errors: [] }
}

/**
 * Assert that a request was captured for the given pattern
 */
export function assertRequestCaptured(
  store: RequestCaptureStore,
  pattern: ApiPattern
): CapturedRequest {
  const request = store.getLastRequest(pattern)
  expect(request, `Expected request for pattern "${pattern}" to be captured`).toBeDefined()
  return request!
}

/**
 * Assert that the request body contains all expected data (partial match)
 */
export function assertPayloadContains(
  actual: Record<string, unknown> | null,
  expected: Record<string, unknown>
): void {
  expect(actual, 'Request body should not be null').not.toBeNull()

  const result = deepPartialMatch(actual, expected)
  if (!result.matches) {
    const errorMessage = [
      'Payload assertion failed:',
      ...result.errors.map((e) => `  - ${e}`),
      '',
      'Actual payload:',
      JSON.stringify(actual, null, 2)
    ].join('\n')
    expect.soft(result.matches, errorMessage).toBe(true)
  }
}

/**
 * Assert that a captured request body contains expected data
 */
export function assertRequestBodyContains(
  store: RequestCaptureStore,
  pattern: ApiPattern,
  expectedData: Record<string, unknown>
): void {
  const request = assertRequestCaptured(store, pattern)
  assertPayloadContains(request.body, expectedData)
}

/**
 * Assert that a specific field exists in the captured request
 */
export function assertFieldInPayload(
  store: RequestCaptureStore,
  pattern: ApiPattern,
  fieldPath: string,
  expectedValue?: unknown
): void {
  const request = assertRequestCaptured(store, pattern)
  expect(request.body, 'Request body should not be null').not.toBeNull()

  const pathParts = fieldPath.split('.')
  let current: unknown = request.body

  for (let i = 0; i < pathParts.length; i++) {
    const part = pathParts[i]
    if (current === null || current === undefined || typeof current !== 'object') {
      expect.fail(`Field path "${fieldPath}" not found - "${pathParts.slice(0, i + 1).join('.')}" is not an object`)
    }
    current = (current as Record<string, unknown>)[part]
  }

  if (expectedValue !== undefined) {
    expect(current, `Field "${fieldPath}" should equal expected value`).toEqual(expectedValue)
  } else {
    expect(current, `Field "${fieldPath}" should exist in payload`).toBeDefined()
  }
}

/**
 * Assert that SEO fields are correctly included in the payload
 */
export function assertSEOPayload(
  store: RequestCaptureStore,
  pattern: ApiPattern,
  seoPath: string = 'seo'
): void {
  const request = assertRequestCaptured(store, pattern)
  expect(request.body, 'Request body should not be null').not.toBeNull()

  const seoFields = [
    'meta_title',
    'meta_description',
    'keywords',
    'meta_robots',
    'canonical_url',
    'og_title',
    'og_description',
    'og_image'
  ]

  // Get SEO object from payload
  const pathParts = seoPath.split('.')
  let seoObj: unknown = request.body
  for (const part of pathParts) {
    if (seoObj && typeof seoObj === 'object') {
      seoObj = (seoObj as Record<string, unknown>)[part]
    }
  }

  // Check that seo object exists and has expected structure
  expect(seoObj, `SEO object at path "${seoPath}" should exist`).toBeDefined()
  expect(typeof seoObj, `SEO at path "${seoPath}" should be an object`).toBe('object')

  const seoRecord = seoObj as Record<string, unknown>

  // Log which SEO fields are present for debugging
  const presentFields = seoFields.filter((field) => field in seoRecord && seoRecord[field] !== undefined)
  const missingFields = seoFields.filter((field) => !(field in seoRecord) || seoRecord[field] === undefined)

  if (missingFields.length > 0) {
    console.log(`SEO fields present: ${presentFields.join(', ')}`)
    console.log(`SEO fields missing: ${missingFields.join(', ')}`)
  }
}

/**
 * Get all captured requests for debugging
 */
export function logCapturedRequests(store: RequestCaptureStore): void {
  const patterns = store.getPatterns()
  console.log('Captured requests:')
  for (const pattern of patterns) {
    const requests = store.getAllRequests(pattern)
    console.log(`  ${pattern}: ${requests.length} request(s)`)
    for (const req of requests) {
      console.log(`    - ${req.method} ${req.url}`)
      if (req.body) {
        console.log(`      Body: ${JSON.stringify(req.body, null, 2).substring(0, 500)}...`)
      }
    }
  }
}

/**
 * Assert no request was captured for a pattern (useful for negative tests)
 */
export function assertNoRequestCaptured(store: RequestCaptureStore, pattern: ApiPattern): void {
  const count = store.getRequestCount(pattern)
  expect(count, `Expected no requests for pattern "${pattern}" but found ${count}`).toBe(0)
}

/**
 * Get the request count for a pattern
 */
export function getRequestCount(store: RequestCaptureStore, pattern: ApiPattern): number {
  return store.getRequestCount(pattern)
}
