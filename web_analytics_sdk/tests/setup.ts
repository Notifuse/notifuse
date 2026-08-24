/**
 * Vitest test setup file
 * Sets up jsdom environment and global mocks
 */

import { vi } from 'vitest';

// Mock __SDK_VERSION__ global. Must keep the real shape — vMAJOR.minor, read
// from config/config.go at build time — so tests cannot assume semver.
(globalThis as unknown as Record<string, string>).__SDK_VERSION__ = '39.0';

// Mock navigator.sendBeacon
Object.defineProperty(navigator, 'sendBeacon', {
  writable: true,
  value: vi.fn(() => true),
});

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] || null,
    setItem: (key: string, value: string) => {
      store[key] = value;
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      store = {};
    },
  };
})();

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
});

// Mock sessionStorage
const sessionStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] || null,
    setItem: (key: string, value: string) => {
      store[key] = value;
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      store = {};
    },
  };
})();

Object.defineProperty(window, 'sessionStorage', {
  value: sessionStorageMock,
});

// Mock fetch
global.fetch = vi.fn(() =>
  Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ success: true }),
  })
) as unknown as typeof fetch;

// Reset mocks before each test
beforeEach(() => {
  vi.clearAllMocks();
  localStorageMock.clear();
  sessionStorageMock.clear();
});
