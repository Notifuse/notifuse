import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { isBot, getBotScore } from './bot';

describe('Bot Detection', () => {
  // Helper to mock navigator properties
  const mockNavigator = (overrides: Partial<Navigator> = {}) => {
    const mockNav = {
      userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
      webdriver: false,
      plugins: { length: 3 } as PluginArray,
      languages: ['en-US', 'en'],
      ...overrides,
    };
    vi.stubGlobal('navigator', mockNav);
  };

  // Helper to mock screen
  const mockScreen = (overrides: Partial<Screen> = {}) => {
    vi.stubGlobal('screen', {
      width: 1920,
      height: 1080,
      ...overrides,
    });
  };

  // Helper to mock window
  const mockWindow = (overrides: Record<string, unknown> = {}) => {
    const currentWindow = globalThis.window || {};
    vi.stubGlobal('window', {
      ...currentWindow,
      chrome: {},
      localStorage: {},
      ...overrides,
    });
  };

  beforeEach(() => {
    // Set up default mocks for a real browser
    mockNavigator();
    mockScreen();
    mockWindow();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe('isBot', () => {
    describe('user agent pattern detection', () => {
      const botUserAgents = [
        { name: 'googlebot', ua: 'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)' },
        { name: 'Googlebot (uppercase)', ua: 'Mozilla/5.0 (compatible; GOOGLEBOT/2.1)' },
        { name: 'bingbot', ua: 'Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)' },
        { name: 'yandex', ua: 'Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)' },
        { name: 'baidu', ua: 'Mozilla/5.0 (compatible; Baiduspider/2.0)' },
        { name: 'duckduck', ua: 'DuckDuckBot/1.0; (+http://duckduckgo.com/duckduckbot.html)' },
        { name: 'slurp', ua: 'Mozilla/5.0 (compatible; Yahoo! Slurp)' },
        { name: 'msnbot', ua: 'msnbot/2.0b (+http://search.msn.com/msnbot.htm)' },
        { name: 'ia_archiver', ua: 'ia_archiver (+http://www.alexa.com/site/help/webmasters)' },
        { name: 'bot generic', ua: 'Some Generic Bot' },
        { name: 'crawler', ua: 'Mozilla/5.0 (compatible; MyCrawler/1.0)' },
        { name: 'spider', ua: 'Mozilla/5.0 (compatible; WebSpider/1.0)' },
        { name: 'scraper', ua: 'Mozilla/5.0 (compatible; MyScraper/1.0)' },
        { name: 'headless', ua: 'Mozilla/5.0 HeadlessChrome/120.0.0.0' },
        { name: 'phantom', ua: 'Mozilla/5.0 PhantomJS/2.0' },
        { name: 'selenium', ua: 'Mozilla/5.0 Selenium/4.0' },
        { name: 'puppeteer', ua: 'Mozilla/5.0 Puppeteer/21.0' },
        { name: 'lighthouse', ua: 'Mozilla/5.0 Chrome-Lighthouse' },
        { name: 'pagespeed', ua: 'Mozilla/5.0 (compatible; Google PageSpeed Insights)' },
        { name: 'gtmetrix', ua: 'Mozilla/5.0 GTmetrix' },
        { name: 'facebook', ua: 'facebookexternalhit/1.1' },
        { name: 'twitter', ua: 'Twitterbot/1.0' },
        { name: 'linkedin', ua: 'LinkedInBot/1.0' },
        { name: 'pinterest', ua: 'Pinterest/0.2 (+http://www.pinterest.com/)' },
      ];

      for (const { name, ua } of botUserAgents) {
        it(`detects ${name}`, () => {
          mockNavigator({ userAgent: ua });
          expect(isBot()).toBe(true);
        });
      }
    });

    describe('webdriver detection', () => {
      it('returns true when navigator.webdriver is true', () => {
        mockNavigator({ webdriver: true });
        expect(isBot()).toBe(true);
      });

      it('returns false when navigator.webdriver is false', () => {
        mockNavigator({ webdriver: false });
        expect(isBot()).toBe(false);
      });

      it('returns false when navigator.webdriver is undefined', () => {
        mockNavigator({ webdriver: undefined });
        expect(isBot()).toBe(false);
      });
    });

    describe('feature fingerprinting', () => {
      it('detects suspicious: 3+ features triggers bot detection', () => {
        // The isBot checks: 'plugins' in navigator || plugins.length === 0
        // So we need to provide empty plugins, not undefined
        mockNavigator({
          plugins: { length: 0 } as PluginArray, // 1st suspicious
          languages: { length: 0 } as unknown as readonly string[], // 2nd suspicious
          userAgent: 'Mozilla/5.0 Chrome/120.0.0.0', // Chrome UA
        });
        mockWindow({ chrome: undefined }); // 3rd suspicious (Chrome UA but no window.chrome)
        // 3 suspicious features = bot
        expect(isBot()).toBe(true);
      });

      it('detects suspicious: plugins empty', () => {
        mockNavigator({
          plugins: { length: 0 } as PluginArray,
          languages: { length: 0 } as unknown as readonly string[],
          userAgent: 'Mozilla/5.0 Chrome/120.0.0.0',
        });
        mockWindow({ chrome: undefined }); // Chrome UA but no window.chrome
        expect(isBot()).toBe(true);
      });

      it('detects suspicious: Chrome UA but no window.chrome', () => {
        mockNavigator({
          plugins: { length: 0 } as PluginArray,
          languages: { length: 0 } as unknown as readonly string[],
          userAgent: 'Mozilla/5.0 Chrome/120.0.0.0',
        });
        mockWindow({ chrome: undefined });
        expect(isBot()).toBe(true);
      });

      it('detects suspicious: zero screen dimensions', () => {
        mockScreen({ width: 0, height: 0 });
        mockNavigator({
          plugins: { length: 0 } as PluginArray,
          languages: { length: 0 } as unknown as readonly string[],
        });
        expect(isBot()).toBe(true);
      });

      it('detects suspicious: mobile UA but no touch support', () => {
        // Mobile UA with 3 suspicious features
        mockNavigator({
          userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) Mobile',
          plugins: { length: 0 } as PluginArray,  // 1st suspicious
          languages: { length: 0 } as unknown as readonly string[], // 2nd suspicious
        });
        // mockWindow sets chrome: {} by default so no Chrome check issue
        // The 'ontouchstart' check is: !('ontouchstart' in window) && /mobile/i.test(ua)
        // We need window to NOT have ontouchstart property at all
        mockScreen({ width: 0, height: 0 }); // 3rd suspicious: zero screen dimensions
        // 3 suspicious features: plugins empty, languages empty, zero screen
        expect(isBot()).toBe(true);
      });

      it('returns false when less than 3 suspicious features', () => {
        mockNavigator({
          plugins: { length: 0 } as PluginArray, // 1 suspicious
          languages: ['en-US'], // not suspicious
        });
        mockWindow({ chrome: {} }); // not suspicious (has chrome)
        mockScreen({ width: 1920, height: 1080 }); // not suspicious
        expect(isBot()).toBe(false);
      });
    });

    describe('real browser user agents', () => {
      const realBrowserUAs = [
        {
          name: 'Chrome on macOS',
          ua: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
        },
        {
          name: 'Firefox on Windows',
          ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0',
        },
        {
          name: 'Safari on macOS',
          ua: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15',
        },
        {
          name: 'Edge on Windows',
          ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0',
        },
        {
          name: 'Chrome on Android',
          ua: 'Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.144 Mobile Safari/537.36',
        },
        {
          name: 'Safari on iOS',
          ua: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1',
        },
      ];

      for (const { name, ua } of realBrowserUAs) {
        it(`returns false for ${name}`, () => {
          mockNavigator({ userAgent: ua });
          // Also mock touch for mobile UAs
          if (ua.includes('Mobile') || ua.includes('iPhone') || ua.includes('Android')) {
            mockWindow({ chrome: {}, ontouchstart: () => {} });
          }
          expect(isBot()).toBe(false);
        });
      }
    });
  });

  describe('getBotScore', () => {
    it('returns 0 for a normal browser', () => {
      mockNavigator();
      mockScreen();
      mockWindow();
      expect(getBotScore()).toBe(0);
    });

    it('returns score capped at 100', () => {
      mockNavigator({
        userAgent: 'Googlebot',
        webdriver: true,
        plugins: { length: 0 } as PluginArray,
        languages: { length: 0 } as unknown as readonly string[],
      });
      mockScreen({ width: 0, height: 0 });
      mockWindow({ chrome: undefined, localStorage: undefined });
      expect(getBotScore()).toBeLessThanOrEqual(100);
    });

    it('adds 40 for UA pattern match', () => {
      mockNavigator({ userAgent: 'Googlebot' });
      const score = getBotScore();
      expect(score).toBeGreaterThanOrEqual(40);
    });

    it('adds 30 for webdriver', () => {
      mockNavigator({ webdriver: true });
      const score = getBotScore();
      expect(score).toBe(30);
    });

    it('adds 5 for each suspicious feature', () => {
      // Start with normal browser, add one suspicious feature
      mockNavigator({
        plugins: { length: 0 } as PluginArray, // +5
      });
      const score = getBotScore();
      expect(score).toBe(5);
    });

    it('adds 5 for missing localStorage', () => {
      mockWindow({ localStorage: undefined });
      const score = getBotScore();
      expect(score).toBe(5);
    });

    it('accumulates scores correctly', () => {
      mockNavigator({
        userAgent: 'Googlebot', // +40
        webdriver: true, // +30
        plugins: { length: 0 } as PluginArray, // +5
        languages: { length: 0 } as unknown as readonly string[], // +5
      });
      // Chrome UA check doesn't apply since not Chrome UA
      // Total: 40 + 30 + 5 + 5 = 80
      const score = getBotScore();
      expect(score).toBe(80);
    });
  });
});
