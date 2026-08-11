import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { SessionState } from './session-state';
import type { SessionAttributes } from '../types/session-state';

describe('SessionState - Payload with identity and dimensions', () => {
  let sessionState: SessionState;
  const mockAttributes: SessionAttributes = {
    landing_page: 'https://example.com/',
    referrer: 'https://google.com',
    utm_source: 'google',
    utm_medium: 'cpc',
    screen_width: 1920,
    screen_height: 1080,
    viewport_width: 1200,
    viewport_height: 800,
    device: 'desktop',
    browser: 'Chrome',
    browser_type: 'chromium',
    os: 'macOS',
    user_agent: 'Mozilla/5.0...',
    connection_type: 'wifi',
    language: 'en-US',
    timezone: 'America/Los_Angeles',
  };

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-15T12:00:00.000Z'));

    sessionState = new SessionState({
      workspace_id: 'ws_123',
      session_id: 'sess_abc',
      created_at: Date.now(),
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('buildPayload includes the verified identity', () => {
    it('includes the signed address when provided', () => {
      const payload = sessionState.buildPayload(mockAttributes, {
        identity: { email: 'user_123', hmac: 'sig' },
      });

      expect(payload.contact_email).toBe('user_123')
      expect(payload.contact_email_hmac).toBe('sig');
    });

    it('omits identity fields when anonymous', () => {
      const payload = sessionState.buildPayload(mockAttributes, {
        identity: null,
      });

      expect(payload.contact_email).toBeUndefined();
    });

    it('omits identity fields when not provided in options', () => {
      const payload = sessionState.buildPayload(mockAttributes);

      expect(payload).not.toHaveProperty('user_id');
    });
  });

  describe('buildPayload includes dimensions', () => {
    it('includes dimensions when provided', () => {
      const dimensions = {
        custom_1: 'value1',
        custom_2: 'value2',
        custom_5: 'value5',
      };

      const payload = sessionState.buildPayload(mockAttributes, {
        dimensions,
      });

      expect(payload.dimensions).toEqual(dimensions);
    });

    it('includes all 10 dimensions when set', () => {
      const dimensions: Record<string, string> = {};
      for (let i = 1; i <= 10; i++) {
        dimensions[`custom_${i}`] = `value_${i}`;
      }

      const payload = sessionState.buildPayload(mockAttributes, {
        dimensions,
      });

      expect(payload.dimensions).toEqual(dimensions);
      expect(Object.keys(payload.dimensions!).length).toBe(10);
    });

    it('includes empty dimensions object when no dimensions set', () => {
      const payload = sessionState.buildPayload(mockAttributes, {
        dimensions: {},
      });

      expect(payload.dimensions).toEqual({});
    });

    it('omits dimensions when not provided in options', () => {
      const payload = sessionState.buildPayload(mockAttributes);

      expect(payload).not.toHaveProperty('dimensions');
    });
  });

  describe('buildPayload includes both identity and dimensions', () => {
    it('includes both identity and dimensions when both provided', () => {
      const dimensions = {
        custom_1: 'campaign_a',
        custom_2: 'variant_b',
      };

      const payload = sessionState.buildPayload(mockAttributes, {
        identity: { email: 'user_xyz', hmac: 'sig' },
        dimensions,
      });

      expect(payload.contact_email).toBe('user_xyz')
      expect(payload.contact_email_hmac).toBe('sig');
      expect(payload.dimensions).toEqual(dimensions);
    });

    it('includes dimensions while anonymous', () => {
      const dimensions = {
        custom_1: 'campaign_a',
      };

      const payload = sessionState.buildPayload(mockAttributes, {
        identity: null,
        dimensions,
      });

      expect(payload.contact_email).toBeUndefined();
      expect(payload.dimensions).toEqual(dimensions);
    });
  });

  describe('payload structure', () => {
    it('maintains existing payload fields alongside new fields', () => {
      sessionState.addPageview('/test-page');

      const payload = sessionState.buildPayload(mockAttributes, {
        identity: { email: 'user_test', hmac: 'sig' },
        dimensions: { custom_1: 'test' },
      });

      // Existing fields
      expect(payload.workspace_id).toBe('ws_123');
      expect(payload.session_id).toBe('sess_abc');
      expect(payload.actions).toHaveLength(1);
      expect(payload.attributes).toBeDefined();
      expect(payload.created_at).toBeDefined();
      expect(payload.updated_at).toBeDefined();
      expect(payload.sdk_version).toBeDefined();

      // New fields
      expect(payload.contact_email).toBe('user_test')
      expect(payload.contact_email_hmac).toBe('sig');
      expect(payload.dimensions).toEqual({ custom_1: 'test' });
    });
  });
});
