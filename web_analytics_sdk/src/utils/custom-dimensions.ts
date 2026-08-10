/**
 * Custom dimension URL parameter parsing
 * Parses custom_1 through custom_10 from URL
 */

import type { CustomDimensions } from '../types';

const MIN_INDEX = 1;
const MAX_INDEX = 10;
const MAX_LENGTH = 256;

/**
 * Parse custom_1 through custom_10 parameters from URL
 * Returns only valid dimensions (string values, max 256 chars)
 */
export function parseCustomDimensions(url: string): CustomDimensions {
  const dimensions: CustomDimensions = {};

  try {
    const params = new URL(url).searchParams;

    for (let i = MIN_INDEX; i <= MAX_INDEX; i++) {
      const value = params.get(`custom_${i}`);
      if (value !== null && value.length <= MAX_LENGTH) {
        dimensions[i] = value;
      }
    }
  } catch {
    // Invalid URL, return empty dimensions
  }

  return dimensions;
}
