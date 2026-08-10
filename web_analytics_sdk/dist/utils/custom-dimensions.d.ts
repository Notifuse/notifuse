/**
 * Custom dimension URL parameter parsing
 * Parses custom_1 through custom_10 from URL
 */
import type { CustomDimensions } from '../types';
/**
 * Parse custom_1 through custom_10 parameters from URL
 * Returns only valid dimensions (string values, max 256 chars)
 */
export declare function parseCustomDimensions(url: string): CustomDimensions;
