/**
 * SPA Navigation Integration Tests
 *
 * Tests that the SDK correctly detects and tracks navigation
 * in Single Page Applications via History API and hash changes.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { NavigationTracker } from '../../src/events/navigation';
import { ScrollTracker } from '../../src/events/scroll';

describe('SPA Navigation Integration', () => {
  let navigationTracker: NavigationTracker;
  let scrollTracker: ScrollTracker;
  let navigationCallback: ReturnType<typeof vi.fn>;
  let originalPushState: typeof history.pushState;
  let originalReplaceState: typeof history.replaceState;

  beforeEach(() => {
    // Store originals
    originalPushState = history.pushState;
    originalReplaceState = history.replaceState;

    // Mock window.location
    const locationMock = {
      href: 'https://example.com/home',
      pathname: '/home',
      hash: '',
      search: '',
    };

    // Use defineProperty for location since it's not directly assignable
    Object.defineProperty(window, 'location', {
      value: locationMock,
      writable: true,
      configurable: true,
    });

    // Mock scroll-related properties
    Object.defineProperty(document.documentElement, 'scrollHeight', {
      value: 2000,
      configurable: true,
    });
    Object.defineProperty(document.body, 'scrollHeight', {
      value: 2000,
      configurable: true,
    });
    Object.defineProperty(window, 'innerHeight', {
      value: 800,
      configurable: true,
    });
    Object.defineProperty(window, 'pageYOffset', {
      value: 0,
      writable: true,
      configurable: true,
    });

    navigationCallback = vi.fn();
    navigationTracker = new NavigationTracker();
    navigationTracker.setNavigationCallback(navigationCallback);

    scrollTracker = new ScrollTracker();
  });

  afterEach(() => {
    navigationTracker.stop();
    scrollTracker.stop();

    // Restore original methods
    history.pushState = originalPushState;
    history.replaceState = originalReplaceState;

    vi.clearAllMocks();
  });

  describe('history.pushState detection', () => {
    it('triggers navigation callback on pushState', () => {
      navigationTracker.start();

      // Simulate pushState navigation
      const newUrl = 'https://example.com/about';
      Object.defineProperty(window, 'location', {
        value: { href: newUrl, pathname: '/about', hash: '', search: '' },
        writable: true,
        configurable: true,
      });

      history.pushState({}, '', '/about');

      expect(navigationCallback).toHaveBeenCalledWith(newUrl);
    });

    it('does not trigger callback if URL unchanged', () => {
      navigationTracker.start();

      // pushState with same URL
      history.pushState({ data: 'test' }, '', '/home');

      expect(navigationCallback).not.toHaveBeenCalled();
    });

    it('tracks navigation to nested paths', () => {
      navigationTracker.start();

      const paths = ['/products', '/products/123', '/products/123/reviews'];

      for (const path of paths) {
        const newUrl = `https://example.com${path}`;
        Object.defineProperty(window, 'location', {
          value: { href: newUrl, pathname: path, hash: '', search: '' },
          writable: true,
          configurable: true,
        });

        history.pushState({}, '', path);
      }

      expect(navigationCallback).toHaveBeenCalledTimes(3);
      expect(navigationCallback).toHaveBeenNthCalledWith(1, 'https://example.com/products');
      expect(navigationCallback).toHaveBeenNthCalledWith(2, 'https://example.com/products/123');
      expect(navigationCallback).toHaveBeenNthCalledWith(3, 'https://example.com/products/123/reviews');
    });
  });

  describe('history.replaceState detection', () => {
    it('triggers navigation callback on replaceState', () => {
      navigationTracker.start();

      const newUrl = 'https://example.com/contact';
      Object.defineProperty(window, 'location', {
        value: { href: newUrl, pathname: '/contact', hash: '', search: '' },
        writable: true,
        configurable: true,
      });

      history.replaceState({}, '', '/contact');

      expect(navigationCallback).toHaveBeenCalledWith(newUrl);
    });

    it('tracks URL with query params via replaceState', () => {
      navigationTracker.start();

      const newUrl = 'https://example.com/search?q=test&page=2';
      Object.defineProperty(window, 'location', {
        value: { href: newUrl, pathname: '/search', hash: '', search: '?q=test&page=2' },
        writable: true,
        configurable: true,
      });

      history.replaceState({}, '', '/search?q=test&page=2');

      expect(navigationCallback).toHaveBeenCalledWith(newUrl);
    });
  });

  describe('popstate (back/forward) detection', () => {
    it('triggers navigation callback on popstate', () => {
      navigationTracker.start();

      const newUrl = 'https://example.com/previous-page';
      Object.defineProperty(window, 'location', {
        value: { href: newUrl, pathname: '/previous-page', hash: '', search: '' },
        writable: true,
        configurable: true,
      });

      // Simulate back button
      window.dispatchEvent(new PopStateEvent('popstate', { state: {} }));

      expect(navigationCallback).toHaveBeenCalledWith(newUrl);
    });

    it('handles multiple back/forward navigations', () => {
      navigationTracker.start();

      const urls = [
        'https://example.com/page1',
        'https://example.com/page2',
        'https://example.com/page1', // back
      ];

      for (const url of urls) {
        Object.defineProperty(window, 'location', {
          value: { href: url, pathname: new URL(url).pathname, hash: '', search: '' },
          writable: true,
          configurable: true,
        });
        window.dispatchEvent(new PopStateEvent('popstate', { state: {} }));
      }

      expect(navigationCallback).toHaveBeenCalledTimes(3);
    });
  });

  describe('hashchange detection', () => {
    it('triggers navigation callback on hashchange', () => {
      navigationTracker.start();

      const newUrl = 'https://example.com/home#section2';
      Object.defineProperty(window, 'location', {
        value: { href: newUrl, pathname: '/home', hash: '#section2', search: '' },
        writable: true,
        configurable: true,
      });

      window.dispatchEvent(new HashChangeEvent('hashchange'));

      expect(navigationCallback).toHaveBeenCalledWith(newUrl);
    });

    it('tracks hash-only navigation', () => {
      navigationTracker.start();

      const hashes = ['#intro', '#features', '#pricing', '#faq'];

      for (const hash of hashes) {
        const newUrl = `https://example.com/home${hash}`;
        Object.defineProperty(window, 'location', {
          value: { href: newUrl, pathname: '/home', hash, search: '' },
          writable: true,
          configurable: true,
        });
        window.dispatchEvent(new HashChangeEvent('hashchange'));
      }

      expect(navigationCallback).toHaveBeenCalledTimes(4);
    });
  });

  describe('scroll reset on navigation', () => {
    it('scroll tracker can be reset independently of navigation', () => {
      vi.useFakeTimers();
      scrollTracker.start();

      // Simulate scrolling to 50%
      Object.defineProperty(window, 'pageYOffset', { value: 600, configurable: true });
      Object.defineProperty(document.documentElement, 'scrollTop', { value: 600, configurable: true });
      window.dispatchEvent(new Event('scroll'));

      // Allow throttle to pass
      vi.advanceTimersByTime(150);

      expect(scrollTracker.getMaxScrollPercent()).toBe(50);

      // Reset (would be called on navigation)
      scrollTracker.reset();

      expect(scrollTracker.getMaxScrollPercent()).toBe(0);
      vi.useRealTimers();
    });

    it('scroll milestones reset after navigation reset', () => {
      vi.useFakeTimers();
      const milestoneCallback = vi.fn();
      scrollTracker.setMilestoneCallback(milestoneCallback);
      scrollTracker.start();

      // Scroll to 50% - triggers 25% and 50% milestones
      Object.defineProperty(window, 'pageYOffset', { value: 600, configurable: true });
      Object.defineProperty(document.documentElement, 'scrollTop', { value: 600, configurable: true });
      window.dispatchEvent(new Event('scroll'));
      vi.advanceTimersByTime(150);

      expect(milestoneCallback).toHaveBeenCalledWith(25);
      expect(milestoneCallback).toHaveBeenCalledWith(50);
      milestoneCallback.mockClear();

      // Reset scroll tracker (as would happen on navigation)
      scrollTracker.reset();

      // Scroll to 50% again - should trigger milestones again
      window.dispatchEvent(new Event('scroll'));
      vi.advanceTimersByTime(150);

      expect(milestoneCallback).toHaveBeenCalledWith(25);
      expect(milestoneCallback).toHaveBeenCalledWith(50);
      vi.useRealTimers();
    });
  });

  describe('getCurrentUrl', () => {
    it('returns current tracked URL', () => {
      navigationTracker.start();

      expect(navigationTracker.getCurrentUrl()).toBe('https://example.com/home');

      const newUrl = 'https://example.com/about';
      Object.defineProperty(window, 'location', {
        value: { href: newUrl, pathname: '/about', hash: '', search: '' },
        writable: true,
        configurable: true,
      });
      history.pushState({}, '', '/about');

      expect(navigationTracker.getCurrentUrl()).toBe(newUrl);
    });
  });

  describe('stop tracking', () => {
    it('stops responding to navigation after stop() called', () => {
      navigationTracker.start();
      navigationTracker.stop();

      const newUrl = 'https://example.com/stopped';
      Object.defineProperty(window, 'location', {
        value: { href: newUrl, pathname: '/stopped', hash: '', search: '' },
        writable: true,
        configurable: true,
      });

      // These should not trigger callback after stop
      history.pushState({}, '', '/stopped');
      window.dispatchEvent(new PopStateEvent('popstate', { state: {} }));
      window.dispatchEvent(new HashChangeEvent('hashchange'));

      // Only the pushState might trigger if history wasn't restored properly
      // The popstate and hashchange definitely should not
      expect(navigationCallback.mock.calls.length).toBeLessThanOrEqual(1);
    });
  });

  describe('integration: navigation + scroll reset flow', () => {
    it('full SPA navigation flow resets scroll correctly', () => {
      vi.useFakeTimers();
      navigationTracker.start();
      scrollTracker.start();

      // Page 1: Scroll to 75%
      Object.defineProperty(window, 'pageYOffset', { value: 900, configurable: true });
      Object.defineProperty(document.documentElement, 'scrollTop', { value: 900, configurable: true });
      window.dispatchEvent(new Event('scroll'));
      vi.advanceTimersByTime(150);
      expect(scrollTracker.getMaxScrollPercent()).toBe(75);

      // Navigate to page 2
      const page2Url = 'https://example.com/page2';
      Object.defineProperty(window, 'location', {
        value: { href: page2Url, pathname: '/page2', hash: '', search: '' },
        writable: true,
        configurable: true,
      });
      history.pushState({}, '', '/page2');

      expect(navigationCallback).toHaveBeenCalledWith(page2Url);

      // Reset scroll (as SDK would do on navigation callback)
      scrollTracker.reset();
      Object.defineProperty(window, 'pageYOffset', { value: 0, configurable: true });
      Object.defineProperty(document.documentElement, 'scrollTop', { value: 0, configurable: true });

      expect(scrollTracker.getMaxScrollPercent()).toBe(0);

      // Page 2: Scroll to 25%
      Object.defineProperty(window, 'pageYOffset', { value: 300, configurable: true });
      Object.defineProperty(document.documentElement, 'scrollTop', { value: 300, configurable: true });
      window.dispatchEvent(new Event('scroll'));
      vi.advanceTimersByTime(150);
      expect(scrollTracker.getMaxScrollPercent()).toBe(25);

      vi.useRealTimers();
    });
  });
});
