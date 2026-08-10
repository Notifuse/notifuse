/**
 * UTM and Ad Click ID parsing utilities
 */

import type { UTMParams } from '../types';

// Default ad click ID parameters to track
export const DEFAULT_AD_CLICK_IDS = [
  'gclid',     // Google Ads
  'fbclid',    // Facebook/Meta Ads
  'msclkid',   // Microsoft Ads
  'dclid',     // DoubleClick
  'twclid',    // Twitter/X Ads
  'ttclid',    // TikTok Ads
  'li_fat_id', // LinkedIn Ads
  'wbraid',    // Google Ads (iOS)
  'gbraid',    // Google Ads (cross-device)
];

/**
 * Parse UTM parameters from URL
 */
export function parseUTMParams(url: string, adClickIds: string[] = DEFAULT_AD_CLICK_IDS): UTMParams {
  const params = new URL(url).searchParams;

  // Find ad click ID
  let utm_id: string | null = null;
  let utm_id_from: string | null = null;

  for (const param of adClickIds) {
    const value = params.get(param);
    if (value) {
      utm_id = value;
      utm_id_from = param;
      break; // Use first match
    }
  }

  return {
    source: params.get('utm_source'),
    medium: params.get('utm_medium'),
    campaign: params.get('utm_campaign'),
    term: params.get('utm_term'),
    content: params.get('utm_content'),
    id: utm_id,
    id_from: utm_id_from,
  };
}

/**
 * Check if UTM params have any values
 */
export function hasUTMParams(utm: UTMParams): boolean {
  return Boolean(
    utm.source ||
    utm.medium ||
    utm.campaign ||
    utm.term ||
    utm.content ||
    utm.id
  );
}

/**
 * Parse referrer information
 */
export function parseReferrer(referrer: string): { domain: string | null; path: string | null } {
  if (!referrer) {
    return { domain: null, path: null };
  }

  try {
    const url = new URL(referrer);
    return {
      domain: url.hostname,
      path: url.pathname,
    };
  } catch {
    return { domain: null, path: null };
  }
}
