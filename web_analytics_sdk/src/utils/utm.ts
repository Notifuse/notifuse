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
  'epik',      // Pinterest Ads
  'ScCid',     // Snapchat Ads (canonical spelling; matched case-insensitively)
  'rdt_cid',   // Reddit Ads
  'qclid',     // Quora Ads
];

/**
 * Parse UTM parameters from URL
 */
export function parseUTMParams(url: string, adClickIds: string[] = DEFAULT_AD_CLICK_IDS): UTMParams {
  const params = new URL(url).searchParams;

  // Ad networks are inconsistent about the casing of their click ids (Snapchat
  // documents ScCid, plenty of links carry sccid) and URLSearchParams.get is
  // case-sensitive, so an exact lookup silently misses them.
  const byLowerKey = new Map<string, string>();
  for (const [key, value] of params) {
    const lower = key.toLowerCase();
    if (!byLowerKey.has(lower)) byLowerKey.set(lower, value);
  }

  // Find ad click ID
  let utm_id: string | null = null;
  let utm_id_from: string | null = null;

  // Iterating adClickIds rather than the URL's parameters is deliberate: it is
  // what keeps priority OUR order instead of whatever order the network wrote
  // them in. gclid must still win over fbclid when both are present.
  for (const param of adClickIds) {
    const value = byLowerKey.get(param.toLowerCase());
    if (value) {
      utm_id = value;
      // The canonical spelling, not the one seen in the URL: the seeded
      // attribution rules compare utm_id_from with an exact equality, so
      // reporting 'sccid' would attribute the click to nothing.
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
