import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { generateUUIDv4, generateUUIDv7 } from './uuid';

describe('UUID Generation', () => {
  describe('generateUUIDv4', () => {
    it('matches the UUID v4 format', () => {
      const uuid = generateUUIDv4();
      const uuidv4Regex = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
      expect(uuid).toMatch(uuidv4Regex);
    });

    it('has length of 36 characters', () => {
      const uuid = generateUUIDv4();
      expect(uuid.length).toBe(36);
    });

    it('has version digit 4 at position 14', () => {
      const uuid = generateUUIDv4();
      expect(uuid[14]).toBe('4');
    });

    it('has variant digit (8, 9, a, or b) at position 19', () => {
      const uuid = generateUUIDv4();
      expect(['8', '9', 'a', 'b']).toContain(uuid[19]);
    });

    it('generates unique values (1000 samples)', () => {
      const uuids = new Set<string>();
      for (let i = 0; i < 1000; i++) {
        uuids.add(generateUUIDv4());
      }
      expect(uuids.size).toBe(1000);
    });

    describe('fallback implementation', () => {
      let originalRandomUUID: typeof crypto.randomUUID | undefined;

      beforeEach(() => {
        originalRandomUUID = crypto.randomUUID;
        // @ts-expect-error - removing randomUUID to test fallback
        delete crypto.randomUUID;
      });

      afterEach(() => {
        if (originalRandomUUID) {
          crypto.randomUUID = originalRandomUUID;
        }
      });

      it('works when crypto.randomUUID is unavailable', () => {
        const uuid = generateUUIDv4();
        const uuidv4Regex = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
        expect(uuid).toMatch(uuidv4Regex);
      });

      it('generates unique values with fallback (100 samples)', () => {
        const uuids = new Set<string>();
        for (let i = 0; i < 100; i++) {
          uuids.add(generateUUIDv4());
        }
        expect(uuids.size).toBe(100);
      });
    });
  });

  describe('generateUUIDv7', () => {
    it('matches the UUID v7 format', () => {
      const uuid = generateUUIDv7();
      const uuidv7Regex = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
      expect(uuid).toMatch(uuidv7Regex);
    });

    it('has length of 36 characters', () => {
      const uuid = generateUUIDv7();
      expect(uuid.length).toBe(36);
    });

    it('has version digit 7 at position 14', () => {
      const uuid = generateUUIDv7();
      expect(uuid[14]).toBe('7');
    });

    it('has variant digit (8, 9, a, or b) at position 19', () => {
      const uuid = generateUUIDv7();
      expect(['8', '9', 'a', 'b']).toContain(uuid[19]);
    });

    it('contains current timestamp in first 12 hex chars', () => {
      const before = Date.now();
      const uuid = generateUUIDv7();
      const after = Date.now();

      // Extract timestamp from UUID (first 12 hex chars, removing dash)
      const timestampHex = uuid.slice(0, 8) + uuid.slice(9, 13);
      const timestamp = parseInt(timestampHex, 16);

      expect(timestamp).toBeGreaterThanOrEqual(before);
      expect(timestamp).toBeLessThanOrEqual(after);
    });

    it('is time-sortable (earlier timestamp = lower value)', async () => {
      const uuid1 = generateUUIDv7();
      await new Promise((resolve) => setTimeout(resolve, 10));
      const uuid2 = generateUUIDv7();

      // Compare lexicographically - earlier UUID should be "smaller"
      expect(uuid1 < uuid2).toBe(true);
    });

    it('generates unique values within same millisecond', () => {
      const uuids = new Set<string>();
      for (let i = 0; i < 100; i++) {
        uuids.add(generateUUIDv7());
      }
      expect(uuids.size).toBe(100);
    });

    describe('fallback implementation', () => {
      let originalGetRandomValues: typeof crypto.getRandomValues;

      beforeEach(() => {
        originalGetRandomValues = crypto.getRandomValues;
        // @ts-expect-error - removing getRandomValues to test fallback
        delete crypto.getRandomValues;
      });

      afterEach(() => {
        crypto.getRandomValues = originalGetRandomValues;
      });

      it('works when crypto.getRandomValues is unavailable', () => {
        const uuid = generateUUIDv7();
        const uuidv7Regex = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
        expect(uuid).toMatch(uuidv7Regex);
      });

      it('generates unique values with fallback (100 samples)', () => {
        const uuids = new Set<string>();
        for (let i = 0; i < 100; i++) {
          uuids.add(generateUUIDv7());
        }
        expect(uuids.size).toBe(100);
      });
    });
  });
});
