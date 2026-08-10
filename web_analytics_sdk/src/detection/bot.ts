/**
 * Bot and crawler detection
 */

// Chrome window type
declare global {
  interface Window {
    chrome?: unknown;
  }
}

const BOT_PATTERNS = [
  /bot/i,
  /crawler/i,
  /spider/i,
  /scraper/i,
  /googlebot/i,
  /bingbot/i,
  /yandex/i,
  /baidu/i,
  /duckduck/i,
  /slurp/i,
  /msnbot/i,
  /ia_archiver/i,
  /facebook/i,
  /twitter/i,
  /linkedin/i,
  /pinterest/i,
  /headless/i,
  /phantom/i,
  /selenium/i,
  /puppeteer/i,
  /lighthouse/i,
  /pagespeed/i,
  /gtmetrix/i,
];

/**
 * Check if the current user is a bot/crawler
 */
export function isBot(): boolean {
  // Layer 1: User-agent patterns
  const ua = navigator.userAgent.toLowerCase();
  if (BOT_PATTERNS.some((p) => p.test(ua))) {
    return true;
  }

  // Layer 2: WebDriver detection (Selenium, Puppeteer, etc.)
  if (navigator.webdriver) {
    return true;
  }

  // Layer 3: Feature fingerprinting
  const suspiciousFeatures = [
    !('plugins' in navigator) || navigator.plugins.length === 0,
    !('languages' in navigator) || navigator.languages.length === 0,
    // Fake Chrome detection
    !window.chrome && /chrome/i.test(ua),
    // Zero screen dimensions
    screen.width === 0 || screen.height === 0,
    // Fake mobile (no touch but mobile UA)
    !('ontouchstart' in window) && /mobile/i.test(ua),
  ];

  const suspiciousCount = suspiciousFeatures.filter(Boolean).length;
  if (suspiciousCount >= 3) {
    return true;
  }

  return false;
}

/**
 * Get bot confidence score (0-100)
 * Higher = more likely to be a bot
 */
export function getBotScore(): number {
  let score = 0;

  const ua = navigator.userAgent.toLowerCase();

  // UA patterns (+40 max)
  if (BOT_PATTERNS.some((p) => p.test(ua))) {
    score += 40;
  }

  // WebDriver (+30)
  if (navigator.webdriver) {
    score += 30;
  }

  // Suspicious features (+5 each, max 30)
  if (!('plugins' in navigator) || navigator.plugins.length === 0) score += 5;
  if (!('languages' in navigator) || navigator.languages.length === 0) score += 5;
  if (!window.chrome && /chrome/i.test(ua)) score += 5;
  if (screen.width === 0 || screen.height === 0) score += 5;
  if (!('ontouchstart' in window) && /mobile/i.test(ua)) score += 5;
  if (!window.localStorage) score += 5;

  return Math.min(score, 100);
}
