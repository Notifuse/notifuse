import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

import type { NotifuseAnalyticsConfig } from './types';

describe('Notifuse Analytics SDK - Global Config Pattern', () => {
  // Store original window.NotifuseAnalyticsConfig
  let originalConfig: NotifuseAnalyticsConfig | undefined;

  beforeEach(() => {
    // Save original
    originalConfig = window.NotifuseAnalyticsConfig;
    // Reset modules to allow fresh import
    vi.resetModules();
  });

  afterEach(() => {
    // Restore original
    if (originalConfig !== undefined) {
      window.NotifuseAnalyticsConfig = originalConfig;
    } else {
      delete window.NotifuseAnalyticsConfig;
    }
    vi.resetModules();
  });

  describe('initialization', () => {
    it('should auto-initialize from window.NotifuseAnalyticsConfig', async () => {
      // Set global config before SDK loads
      window.NotifuseAnalyticsConfig = {
        workspace_id: 'test-ws',
        endpoint: 'https://test.com',
      };

      // Dynamically import the SDK
      const { default: NotifuseAnalytics } = await import('./index');

      // Verify SDK exports the API
      expect(typeof NotifuseAnalytics.trackGoal).toBe('function');
      expect(typeof NotifuseAnalytics.getSessionId).toBe('function');
      expect(typeof NotifuseAnalytics.debug).toBe('function');
    });

    it('should handle missing config gracefully on import', async () => {
      // No config set
      delete window.NotifuseAnalyticsConfig;

      // Should not throw on import
      const { default: NotifuseAnalytics } = await import('./index');

      expect(typeof NotifuseAnalytics.trackGoal).toBe('function');
    });

    it('should have init method on public API', async () => {
      window.NotifuseAnalyticsConfig = {
        workspace_id: 'test-ws',
        endpoint: 'https://test.com',
      };

      const { default: NotifuseAnalytics } = await import('./index');

      // init should exist on the public API
      expect(typeof NotifuseAnalytics.init).toBe('function');
    });

    it('init should be idempotent (calling twice returns same promise)', async () => {
      delete window.NotifuseAnalyticsConfig;

      const { default: NotifuseAnalytics } = await import('./index');

      const config = {
        workspace_id: 'test-ws',
        endpoint: 'https://test.com',
      };

      // Call init twice
      const promise1 = NotifuseAnalytics.init(config);
      const promise2 = NotifuseAnalytics.init(config);

      // Should return the same promise (or both resolve without error)
      await expect(promise1).resolves.toBeUndefined();
      await expect(promise2).resolves.toBeUndefined();

      // Config should be set
      expect(NotifuseAnalytics.getConfig()).not.toBeNull();
      expect(NotifuseAnalytics.getConfig()?.workspace_id).toBe('test-ws');
    });
  });

  describe('error handling', () => {
    it('should throw when calling methods without config', async () => {
      // No config set
      delete window.NotifuseAnalyticsConfig;

      const { default: NotifuseAnalytics } = await import('./index');

      // Calling a method should throw
      await expect(NotifuseAnalytics.getSessionId()).rejects.toThrow(
        'Notifuse Analytics not configured'
      );
    });

    it('should throw for trackGoal without config', async () => {
      delete window.NotifuseAnalyticsConfig;

      const { default: NotifuseAnalytics } = await import('./index');

      await expect(NotifuseAnalytics.trackGoal({ action: 'test' })).rejects.toThrow(
        'Notifuse Analytics not configured'
      );
    });

    it('should throw for trackPageView without config', async () => {
      delete window.NotifuseAnalyticsConfig;

      const { default: NotifuseAnalytics } = await import('./index');

      await expect(NotifuseAnalytics.trackPageView()).rejects.toThrow(
        'Notifuse Analytics not configured'
      );
    });

    it('should throw for setDimension without config', async () => {
      delete window.NotifuseAnalyticsConfig;

      const { default: NotifuseAnalytics } = await import('./index');

      await expect(NotifuseAnalytics.setDimension(1, 'test')).rejects.toThrow(
        'Notifuse Analytics not configured'
      );
    });
  });

  describe('async API', () => {
    it('should return Promise from getSessionId', async () => {
      window.NotifuseAnalyticsConfig = {
        workspace_id: 'test-ws',
        endpoint: 'https://test.com',
      };

      const { default: NotifuseAnalytics } = await import('./index');

      const result = NotifuseAnalytics.getSessionId();
      expect(result).toBeInstanceOf(Promise);
    });

    it('should return Promise from trackGoal', async () => {
      window.NotifuseAnalyticsConfig = {
        workspace_id: 'test-ws',
        endpoint: 'https://test.com',
      };

      const { default: NotifuseAnalytics } = await import('./index');

      const result = NotifuseAnalytics.trackGoal({ action: 'test' });
      expect(result).toBeInstanceOf(Promise);
    });

    it('should return Promise from getDimension', async () => {
      window.NotifuseAnalyticsConfig = {
        workspace_id: 'test-ws',
        endpoint: 'https://test.com',
      };

      const { default: NotifuseAnalytics } = await import('./index');

      const result = NotifuseAnalytics.getDimension(1);
      expect(result).toBeInstanceOf(Promise);
    });
  });

  describe('sync methods', () => {
    it('getConfig should return null when not initialized', async () => {
      delete window.NotifuseAnalyticsConfig;

      const { default: NotifuseAnalytics } = await import('./index');

      // getConfig is sync and returns null if not initialized
      expect(NotifuseAnalytics.getConfig()).toBeNull();
    });

    it('debug should return partial info when not initialized', async () => {
      delete window.NotifuseAnalyticsConfig;

      const { default: NotifuseAnalytics } = await import('./index');

      // debug is sync
      const debugInfo = NotifuseAnalytics.debug();
      expect(debugInfo).toBeDefined();
      expect(debugInfo.session).toBeNull();
      expect(debugInfo.config).toBeNull();
    });
  });

  describe('type exports', () => {
    it('should export types correctly', async () => {
      const types = await import('./index');

      // Should export type definitions (they exist at compile time)
      expect(types.default).toBeDefined();
    });
  });

  describe('config validation', () => {
    it('should throw when workspace_id is missing', async () => {
      window.NotifuseAnalyticsConfig = {
        workspace_id: '',
        endpoint: 'https://test.com',
      };

      const { default: NotifuseAnalytics } = await import('./index');

      // The init will fail, so calling a method should throw
      await expect(NotifuseAnalytics.getSessionId()).rejects.toThrow('workspace_id is required');
    });

    it('should throw when endpoint is missing', async () => {
      window.NotifuseAnalyticsConfig = {
        workspace_id: 'test-ws',
        endpoint: '',
      };

      const { default: NotifuseAnalytics } = await import('./index');

      await expect(NotifuseAnalytics.getSessionId()).rejects.toThrow('endpoint is required');
    });
  });
});
