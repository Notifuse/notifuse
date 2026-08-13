/**
 * SessionState Integration Tests
 *
 * Tests for V3 session payload cumulative actions array.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { SessionState, SessionStateConfig } from '../../src/core/session-state';
import type { PageviewAction, GoalAction } from '../../src/types/session-state';

describe('SessionState', () => {
  let sessionState: SessionState;
  const mockConfig: SessionStateConfig = {
    workspace_id: 'test-ws',
    session_id: 'sess-123',
    created_at: Date.now() - 10000,
  };

  let mockSessionStorage: ReturnType<typeof createMockStorage>;

  const createMockStorage = () => {
    const store: Record<string, string> = {};
    return {
      getItem: vi.fn((key: string) => store[key] ?? null),
      setItem: vi.fn((key: string, value: string) => {
        store[key] = value;
      }),
      removeItem: vi.fn((key: string) => {
        delete store[key];
      }),
      clear: vi.fn(() => {
        Object.keys(store).forEach((k) => delete store[k]);
      }),
      key: vi.fn((index: number) => Object.keys(store)[index] ?? null),
      get length() {
        return Object.keys(store).length;
      },
      _store: store,
    };
  };

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-15T12:00:00.000Z'));

    mockSessionStorage = createMockStorage();
    vi.stubGlobal('sessionStorage', mockSessionStorage);

    sessionState = new SessionState(mockConfig);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  describe('initial state', () => {
    it('starts with empty actions array', () => {
      expect(sessionState.getActions()).toEqual([]);
    });

    it('starts with null currentPage (no page being viewed)', () => {
      expect(sessionState.getCurrentPage()).toBeNull();
    });
  });

  describe('addPageview - first page', () => {
    it('adds page to actions array immediately with duration=0', () => {
      sessionState.addPageview('/home');

      // Page should be in actions immediately (not waiting for navigation)
      const actions = sessionState.getActions();
      expect(actions).toHaveLength(1);
      expect(actions[0].type).toBe('pageview');

      const pageview = actions[0] as PageviewAction;
      expect(pageview.path).toBe('/home');
      expect(pageview.page_number).toBe(1);
      expect(pageview.scroll).toBe(0);
      expect(pageview.duration).toBe(0); // Initial duration is 0
      expect(pageview.entered_at).toBeGreaterThan(0);
      expect(pageview.exited_at).toBe(pageview.entered_at); // Not exited yet
    });

    it('getCurrentPage returns current page info derived from actions', () => {
      sessionState.addPageview('/home');

      const currentPage = sessionState.getCurrentPage();
      expect(currentPage).not.toBeNull();
      expect(currentPage?.path).toBe('/home');
      expect(currentPage?.page_number).toBe(1);
      expect(currentPage?.scroll).toBe(0);
      expect(currentPage?.entered_at).toBeGreaterThan(0);
    });
  });

  describe('addPageview - navigation', () => {
    beforeEach(() => {
      // First page added to actions with duration=0
      sessionState.addPageview('/home');
      // Simulate time on page (for focus time tracking)
      vi.advanceTimersByTime(5000);
      sessionState.updateScroll(75);
    });

    it('finalizes previous page and adds new page to actions', () => {
      // Set up focus time getter to return 5000ms (simulating focus time)
      sessionState.setFocusTimeGetter(() => 5000);

      sessionState.addPageview('/about');

      const actions = sessionState.getActions();
      // Both pages should be in actions: home (finalized) + about (new)
      expect(actions).toHaveLength(2);

      // First page (home) should have final duration
      const homePage = actions[0] as PageviewAction;
      expect(homePage.path).toBe('/home');
      expect(homePage.page_number).toBe(1);
      expect(homePage.scroll).toBe(75);
      expect(homePage.duration).toBe(5000); // Focus time from getter
      expect(homePage.exited_at).toBeGreaterThan(homePage.entered_at);

      // Second page (about) should have duration=0
      const aboutPage = actions[1] as PageviewAction;
      expect(aboutPage.path).toBe('/about');
      expect(aboutPage.page_number).toBe(2);
      expect(aboutPage.duration).toBe(0); // Just started
    });

    it('sets new currentPage with incremented page_number', () => {
      sessionState.addPageview('/about');

      const currentPage = sessionState.getCurrentPage();
      expect(currentPage?.path).toBe('/about');
      expect(currentPage?.page_number).toBe(2);
      expect(currentPage?.scroll).toBe(0); // Reset for new page
    });

    it('increments page_number for each navigation', () => {
      sessionState.addPageview('/about'); // page 2 added
      sessionState.addPageview('/contact'); // page 3 added
      sessionState.addPageview('/pricing'); // page 4 added

      const actions = sessionState.getActions();
      // All 4 pages should be in actions
      const pageviews = actions.filter(
        (a) => a.type === 'pageview',
      ) as PageviewAction[];
      expect(pageviews.map((p) => p.page_number)).toEqual([1, 2, 3, 4]);

      expect(sessionState.getCurrentPage()?.page_number).toBe(4);
    });
  });

  describe('addGoal', () => {
    beforeEach(() => {
      // addPageview now adds page to actions immediately
      sessionState.addPageview('/checkout');
    });

    it('adds goal action to actions array after pageview', () => {
      sessionState.addGoal('purchase', 'other', 99.99);

      const actions = sessionState.getActions();
      // actions[0] = pageview (checkout), actions[1] = goal
      expect(actions).toHaveLength(2);
      expect(actions[0].type).toBe('pageview');
      expect(actions[1].type).toBe('goal');

      const goal = actions[1] as GoalAction;
      expect(goal.name).toBe('purchase');
      expect(goal.value).toBe(99.99);
      expect(goal.path).toBe('/checkout');
      expect(goal.page_number).toBe(1);
      expect(goal.timestamp).toBeGreaterThan(0);
    });

    it('adds goal with optional properties', () => {
      sessionState.addGoal('signup', 'other', undefined, { plan: 'premium' });

      // Goal is second action after pageview
      const goal = sessionState.getActions()[1] as GoalAction;
      expect(goal.properties).toEqual({ plan: 'premium' });
      expect(goal.value).toBeUndefined();
    });

    it('goal does not affect currentPage index', () => {
      sessionState.addGoal('add_to_cart', 'other', 50);

      // currentPage should still point to checkout
      expect(sessionState.getCurrentPage()?.path).toBe('/checkout');
      // actions = [pageview, goal]
      expect(sessionState.getActions()[0].type).toBe('pageview');
      expect(sessionState.getActions()[1].type).toBe('goal');
    });

    it('multiple goals on same page have same page_number', () => {
      sessionState.addGoal('view_product', 'other');
      sessionState.addGoal('add_to_cart', 'other', 50);
      sessionState.addGoal('begin_checkout', 'other');

      // actions = [pageview, goal, goal, goal]
      const goals = sessionState
        .getActions()
        .filter((a) => a.type === 'goal') as GoalAction[];
      expect(goals).toHaveLength(3);
      expect(goals.every((g) => g.page_number === 1)).toBe(true);
    });
  });

  describe('updateScroll', () => {
    beforeEach(() => {
      sessionState.addPageview('/article');
    });

    it('updates currentPage scroll when higher', () => {
      sessionState.updateScroll(25);
      expect(sessionState.getCurrentPage()?.scroll).toBe(25);

      sessionState.updateScroll(50);
      expect(sessionState.getCurrentPage()?.scroll).toBe(50);

      sessionState.updateScroll(75);
      expect(sessionState.getCurrentPage()?.scroll).toBe(75);
    });

    it('does not decrease scroll (tracks max)', () => {
      sessionState.updateScroll(75);
      sessionState.updateScroll(50); // User scrolled up

      expect(sessionState.getCurrentPage()?.scroll).toBe(75); // Still 75
    });

    it('clamps scroll to 0-100 range', () => {
      sessionState.updateScroll(150);
      expect(sessionState.getCurrentPage()?.scroll).toBe(100);

      sessionState.updateScroll(-10);
      expect(sessionState.getCurrentPage()?.scroll).toBe(100); // Still 100 (max)
    });

    it('no-op if no currentPage', () => {
      const emptyState = new SessionState(mockConfig);
      expect(() => emptyState.updateScroll(50)).not.toThrow();
    });
  });

  describe('buildPayload', () => {
    const mockAttributes = {
      landing_page: 'https://example.com/home',
      referrer: 'https://google.com',
      utm_source: 'google',
      device: 'desktop',
      browser: 'Chrome',
    };

    it('includes all actions in payload (pages added immediately)', () => {
      sessionState.addPageview('/home');
      sessionState.addGoal('signup', 'other');
      sessionState.addPageview('/dashboard');

      const payload = sessionState.buildPayload(mockAttributes);

      // Actions are added in order:
      // 1. /home pageview (with duration=0 initially)
      // 2. signup goal
      // 3. /dashboard pageview (when navigation happens, home is finalized)
      expect(payload.actions).toHaveLength(3);
      expect(payload.actions[0].type).toBe('pageview'); // home
      expect(payload.actions[1].type).toBe('goal'); // signup
      expect(payload.actions[2].type).toBe('pageview'); // dashboard
    });

    it('does NOT include current_page field (page is in actions)', () => {
      sessionState.addPageview('/current');
      sessionState.updateScroll(30);

      const payload = sessionState.buildPayload(mockAttributes);

      // No more current_page field - page is in actions[].
      // Read through an index signature: the field is gone from the type, and
      // the assertion is that it is gone from the wire payload too.
      expect((payload as unknown as Record<string, unknown>).current_page).toBeUndefined();

      // Current page should be in actions with scroll
      const pageview = payload.actions[0] as PageviewAction;
      expect(pageview.path).toBe('/current');
      expect(pageview.scroll).toBe(30);
    });

    it('includes attributes in EVERY payload (no attributesSent optimization)', () => {
      sessionState.addPageview('/home');

      // First payload - includes attributes
      const payload1 = sessionState.buildPayload(mockAttributes);
      expect(payload1.attributes).toBeDefined();
      expect(payload1.attributes?.landing_page).toBe('https://example.com/home');

      // Second payload - STILL includes attributes (no optimization)
      const payload2 = sessionState.buildPayload(mockAttributes);
      expect(payload2.attributes).toBeDefined();
      expect(payload2.attributes?.landing_page).toBe('https://example.com/home');
    });

    it('does NOT include checkpoint field (removed)', () => {
      sessionState.addPageview('/home');

      const payload = sessionState.buildPayload(mockAttributes);

      // No checkpoint field in V3 format
      expect((payload as unknown as Record<string, unknown>).checkpoint).toBeUndefined();
    });

    it('includes session metadata', () => {
      sessionState.addPageview('/test');

      const payload = sessionState.buildPayload(mockAttributes);

      expect(payload.workspace_id).toBe('test-ws');
      expect(payload.session_id).toBe('sess-123');
      expect(payload.created_at).toBe(mockConfig.created_at);
      expect(payload.updated_at).toBeGreaterThan(0);
      expect(payload.sdk_version).toBeDefined();
    });
  });

  describe('finalizeForUnload', () => {
    it('updates duration on current page action and clears currentPageIndex', () => {
      // Set up focus time getter
      sessionState.setFocusTimeGetter(() => 10000);
      sessionState.addPageview('/article');
      sessionState.updateScroll(80);

      sessionState.finalizeForUnload();

      // Page was already in actions, just updated
      const actions = sessionState.getActions();
      expect(actions).toHaveLength(1);
      expect(actions[0].type).toBe('pageview');

      const pageview = actions[0] as PageviewAction;
      expect(pageview.path).toBe('/article');
      expect(pageview.scroll).toBe(80);
      expect(pageview.duration).toBe(10000); // Focus time from getter
    });

    it('clears currentPage after finalize', () => {
      sessionState.addPageview('/test');
      sessionState.finalizeForUnload();

      expect(sessionState.getCurrentPage()).toBeNull();
    });

    it('is idempotent (safe to call multiple times)', () => {
      sessionState.addPageview('/test');

      sessionState.finalizeForUnload();
      sessionState.finalizeForUnload();
      sessionState.finalizeForUnload();

      expect(sessionState.getActions()).toHaveLength(1);
    });

    it('no-op if no currentPage', () => {
      sessionState.finalizeForUnload();

      expect(sessionState.getActions()).toHaveLength(0);
    });
  });

  describe('persistence', () => {
    const STORAGE_KEY = 'nf_session_state';

    it('persist saves state to sessionStorage', () => {
      sessionState.addPageview('/home');
      sessionState.addGoal('test', 'other');

      sessionState.persist();

      const stored = mockSessionStorage.getItem(STORAGE_KEY);
      expect(stored).not.toBeNull();

      const parsed = JSON.parse(stored!);
      // Both pageview and goal in actions (page added immediately)
      expect(parsed.actions).toHaveLength(2);
      expect(parsed.currentPageIndex).toBe(0); // Points to pageview
    });

    it('restore loads state from sessionStorage', () => {
      // Setup initial state
      sessionState.addPageview('/home');
      sessionState.addGoal('signup', 'other');
      sessionState.persist();

      // Create new instance and restore
      const newState = new SessionState(mockConfig);
      newState.restore();

      // Both pageview and goal restored
      expect(newState.getActions()).toHaveLength(2);
      expect(newState.getCurrentPage()?.path).toBe('/home');
    });

    it('restore handles missing storage gracefully', () => {
      mockSessionStorage.clear();

      const newState = new SessionState(mockConfig);
      expect(() => newState.restore()).not.toThrow();
      expect(newState.getActions()).toEqual([]);
    });

    it('restore handles corrupted storage gracefully', () => {
      mockSessionStorage.setItem(STORAGE_KEY, 'not-valid-json');

      const newState = new SessionState(mockConfig);
      expect(() => newState.restore()).not.toThrow();
      expect(newState.getActions()).toEqual([]);
    });

    it('restore validates session_id matches', () => {
      // Store state with actual data
      sessionState.addPageview('/home');
      sessionState.addGoal('test_goal', 'other');
      sessionState.persist();

      // Create state for different session
      const differentSession = new SessionState({
        ...mockConfig,
        session_id: 'different-session',
      });
      differentSession.restore();

      // Should not restore state from different session
      expect(differentSession.getActions()).toEqual([]);
      expect(differentSession.getCurrentPage()).toBeNull();
    });
  });

  describe('MAX_ACTIONS limit', () => {
    const MAX_ACTIONS = 1000;

    it('warns when approaching MAX_ACTIONS limit (90%)', () => {
      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

      // Add actions up to 89% (899 actions, should not warn yet)
      for (let i = 0; i < 899; i++) {
        sessionState.addGoal(`goal_${i}`, 'other');
      }

      // Should not warn at 89%
      // The warning happens at >= 90%, so 900 is the first to trigger
      consoleSpy.mockClear();

      // Add one more to reach exactly 90% threshold (900 actions)
      sessionState.addGoal('goal_900', 'other');

      // Should warn now (900 >= 1000 * 0.9)
      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining('Approaching MAX_ACTIONS limit'),
      );

      consoleSpy.mockRestore();
    });

    it('prevents adding actions beyond MAX_ACTIONS', () => {
      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

      // Add MAX_ACTIONS goals
      for (let i = 0; i < MAX_ACTIONS; i++) {
        sessionState.addGoal(`goal_${i}`, 'other');
      }

      expect(sessionState.getActions()).toHaveLength(MAX_ACTIONS);

      // Try to add one more - should return false
      const result = sessionState.addGoal('over_limit', 'other');

      expect(result).toBe(false);
      // Should still be MAX_ACTIONS (extra action rejected)
      expect(sessionState.getActions()).toHaveLength(MAX_ACTIONS);
      // Should warn about MAX_ACTIONS reached
      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining('MAX_ACTIONS'),
      );

      consoleSpy.mockRestore();
    });

    it('pageview navigations are blocked by MAX_ACTIONS', () => {
      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

      // Fill up with goals
      for (let i = 0; i < MAX_ACTIONS; i++) {
        sessionState.addGoal(`goal_${i}`, 'other');
      }

      // addPageview should NOT work when at limit (pageviews now count toward limit)
      sessionState.addPageview('/important');

      // currentPage should NOT be set (blocked)
      expect(sessionState.getCurrentPage()).toBeNull();

      consoleSpy.mockRestore();
    });

    it('addGoal returns true when under limit, false when at limit', () => {
      // Should return true when under limit
      expect(sessionState.addGoal('goal_1', 'other')).toBe(true);

      // Fill to MAX_ACTIONS - 1
      for (let i = 1; i < MAX_ACTIONS; i++) {
        sessionState.addGoal(`goal_${i}`, 'other');
      }

      // Should return false when at limit
      expect(sessionState.addGoal('over_limit', 'other')).toBe(false);
    });
  });

  describe('focus time via callback', () => {
    it('buildPayload uses focusTimeGetter for current page duration', () => {
      sessionState.setFocusTimeGetter(() => 5000); // 5 seconds focus time
      sessionState.addPageview('/home');

      const payload = sessionState.buildPayload({
        landing_page: 'https://example.com',
      });

      // Current page should have duration from focus time getter
      expect(payload.actions[0].type).toBe('pageview');
      expect((payload.actions[0] as PageviewAction).duration).toBe(5000);
    });

    it('navigation uses focusTimeGetter for previous page final duration', () => {
      sessionState.setFocusTimeGetter(() => 3000);
      sessionState.addPageview('/home');

      // Simulate time passing, update focus getter
      sessionState.setFocusTimeGetter(() => 7000);
      sessionState.addPageview('/about');

      // Previous page (home) should have final duration from getter
      const homePage = sessionState.getActions()[0] as PageviewAction;
      expect(homePage.duration).toBe(7000);

      // New page (about) should start with duration=0
      const aboutPage = sessionState.getActions()[1] as PageviewAction;
      expect(aboutPage.duration).toBe(0);
    });

    it('updates exited_at when building payload', () => {
      const initialTime = Date.now();
      sessionState.setFocusTimeGetter(() => 1000);
      sessionState.addPageview('/test');

      vi.advanceTimersByTime(5000); // Advance time by 5 seconds

      const payload = sessionState.buildPayload({
        landing_page: 'https://example.com',
      });

      const pageview = payload.actions[0] as PageviewAction;
      // exited_at should be updated to current time
      expect(pageview.exited_at).toBeGreaterThan(initialTime);
    });

    it('without focusTimeGetter, uses 0 for duration', () => {
      // No setter called - should default to 0
      sessionState.addPageview('/home');

      const payload = sessionState.buildPayload({
        landing_page: 'https://example.com',
      });

      expect((payload.actions[0] as PageviewAction).duration).toBe(0);
    });
  });
});
