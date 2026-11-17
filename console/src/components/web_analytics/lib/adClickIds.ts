/**
 * The ad click ids the browser SDK looks for by default, in its priority order (the first one
 * present wins). A copy of DEFAULT_AD_CLICK_IDS in web_analytics_sdk/src/utils/utm.ts: the console
 * image is built from console/ alone and cannot import the SDK.
 * internal/domain/web_analytics_clickid_contract_test.go fails when the two lists differ, so keep
 * this a plain array literal of single-quoted strings, as the test parses it.
 */
export const DEFAULT_AD_CLICK_IDS = [
  'gclid', // Google Ads
  'fbclid', // Facebook/Meta Ads
  'msclkid', // Microsoft Ads
  'dclid', // DoubleClick
  'twclid', // Twitter/X Ads
  'ttclid', // TikTok Ads
  'li_fat_id', // LinkedIn Ads
  'wbraid', // Google Ads (iOS)
  'gbraid', // Google Ads (cross-device)
  'epik', // Pinterest Ads
  'ScCid', // Snapchat Ads (canonical spelling; matched case-insensitively)
  'rdt_cid', // Reddit Ads
  'qclid' // Quora Ads
]
