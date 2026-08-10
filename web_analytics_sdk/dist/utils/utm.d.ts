/**
 * UTM and Ad Click ID parsing utilities
 */
import type { UTMParams } from '../types';
export declare const DEFAULT_AD_CLICK_IDS: string[];
/**
 * Parse UTM parameters from URL
 */
export declare function parseUTMParams(url: string, adClickIds?: string[]): UTMParams;
/**
 * Check if UTM params have any values
 */
export declare function hasUTMParams(utm: UTMParams): boolean;
/**
 * Parse referrer information
 */
export declare function parseReferrer(referrer: string): {
    domain: string | null;
    path: string | null;
};
