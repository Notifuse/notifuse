import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { Storage, TabStorage, STORAGE_KEYS } from './storage';

describe('Storage', () => {
  let storage: Storage;

  // Mock localStorage
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
        Object.keys(store).forEach((key) => delete store[key]);
      }),
      key: vi.fn((index: number) => Object.keys(store)[index] ?? null),
      get length() {
        return Object.keys(store).length;
      },
      _store: store,
    };
  };

  let mockLocalStorage: ReturnType<typeof createMockStorage>;

  beforeEach(() => {
    mockLocalStorage = createMockStorage();
    vi.stubGlobal('localStorage', mockLocalStorage);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe('initialization', () => {
    it('uses localStorage by default', () => {
      storage = new Storage();
      expect(storage.isUsingMemory()).toBe(false);
    });

    it('triggers memory fallback when localStorage throws on test', () => {
      mockLocalStorage.setItem.mockImplementation(() => {
        throw new Error('QuotaExceededError');
      });
      storage = new Storage();
      expect(storage.isUsingMemory()).toBe(true);
    });
  });

  describe('get', () => {
    beforeEach(() => {
      storage = new Storage();
    });

    it('returns parsed JSON', () => {
      mockLocalStorage._store['nf_test'] = JSON.stringify({ foo: 'bar' });
      const result = storage.get<{ foo: string }>('test');
      expect(result).toEqual({ foo: 'bar' });
    });

    it('returns null for missing key', () => {
      const result = storage.get('nonexistent');
      expect(result).toBeNull();
    });

    it('returns null for invalid JSON', () => {
      mockLocalStorage._store['nf_test'] = 'not valid json {{{';
      mockLocalStorage.getItem.mockReturnValue('not valid json {{{');
      const result = storage.get('test');
      expect(result).toBeNull();
    });

    it('switches to memory on localStorage error mid-session', () => {
      mockLocalStorage.getItem.mockImplementation(() => {
        throw new Error('Storage error');
      });
      const result = storage.get('test');
      expect(result).toBeNull();
      expect(storage.isUsingMemory()).toBe(true);
    });

    it('uses key prefix nf_', () => {
      storage.get('session');
      expect(mockLocalStorage.getItem).toHaveBeenCalledWith('nf_session');
    });
  });

  describe('set', () => {
    beforeEach(() => {
      storage = new Storage();
    });

    it('stores JSON.stringify(value)', () => {
      storage.set('test', { foo: 'bar' });
      expect(mockLocalStorage.setItem).toHaveBeenCalledWith(
        'nf_test',
        JSON.stringify({ foo: 'bar' })
      );
    });

    it('uses key prefix nf_', () => {
      storage.set('session', { id: '123' });
      expect(mockLocalStorage.setItem).toHaveBeenCalledWith(
        'nf_session',
        expect.any(String)
      );
    });

    it('switches to memory on QuotaExceededError', () => {
      mockLocalStorage.setItem.mockImplementation(() => {
        throw new Error('QuotaExceededError');
      });
      storage = new Storage(); // Will use memory due to test failure
      // Verify it's using memory
      storage.set('test', { foo: 'bar' });
      expect(storage.isUsingMemory()).toBe(true);
      // Can still retrieve the value
      const result = storage.get<{ foo: string }>('test');
      expect(result).toEqual({ foo: 'bar' });
    });
  });

  describe('remove', () => {
    beforeEach(() => {
      storage = new Storage();
    });

    it('deletes from localStorage', () => {
      storage.remove('test');
      expect(mockLocalStorage.removeItem).toHaveBeenCalledWith('nf_test');
    });

    it('deletes from memory when in fallback mode', () => {
      mockLocalStorage.setItem.mockImplementation(() => {
        throw new Error('QuotaExceededError');
      });
      storage = new Storage();
      storage.set('test', 'value');
      expect(storage.get('test')).toBe('value');
      storage.remove('test');
      expect(storage.get('test')).toBeNull();
    });
  });

  describe('clear', () => {
    beforeEach(() => {
      storage = new Storage();
    });

    it('removes only keys starting with nf_', () => {
      mockLocalStorage._store['nf_session'] = '"session_data"';
      mockLocalStorage._store['nf_pending'] = '"pending_data"';
      mockLocalStorage._store['other_key'] = '"should_remain"';

      storage.clear();

      expect(mockLocalStorage.removeItem).toHaveBeenCalledWith('nf_session');
      expect(mockLocalStorage.removeItem).toHaveBeenCalledWith('nf_pending');
      expect(mockLocalStorage.removeItem).not.toHaveBeenCalledWith('other_key');
    });

    it('preserves non-nf_ keys', () => {
      mockLocalStorage._store['other_app_data'] = 'preserved';
      storage.clear();
      expect(mockLocalStorage._store['other_app_data']).toBe('preserved');
    });
  });

  describe('isUsingMemory', () => {
    it('returns false when using localStorage', () => {
      storage = new Storage();
      expect(storage.isUsingMemory()).toBe(false);
    });

    it('returns true when in fallback mode', () => {
      mockLocalStorage.setItem.mockImplementation(() => {
        throw new Error('QuotaExceededError');
      });
      storage = new Storage();
      expect(storage.isUsingMemory()).toBe(true);
    });
  });

  describe('memory fallback', () => {
    it('persists data correctly in memory', () => {
      mockLocalStorage.setItem.mockImplementation(() => {
        throw new Error('QuotaExceededError');
      });
      storage = new Storage();

      storage.set('key1', { data: 'value1' });
      storage.set('key2', 'string_value');
      storage.set('key3', 123);

      expect(storage.get<{ data: string }>('key1')).toEqual({ data: 'value1' });
      expect(storage.get<string>('key2')).toBe('string_value');
      expect(storage.get<number>('key3')).toBe(123);
    });
  });
});

describe('TabStorage', () => {
  let tabStorage: TabStorage;

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
      clear: vi.fn(),
      key: vi.fn((index: number) => Object.keys(store)[index] ?? null),
      get length() {
        return Object.keys(store).length;
      },
      _store: store,
    };
  };

  let mockSessionStorage: ReturnType<typeof createMockStorage>;

  beforeEach(() => {
    mockSessionStorage = createMockStorage();
    vi.stubGlobal('sessionStorage', mockSessionStorage);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe('initialization', () => {
    it('uses sessionStorage by default', () => {
      tabStorage = new TabStorage();
      tabStorage.set('test', 'value');
      expect(mockSessionStorage.setItem).toHaveBeenCalled();
    });

    it('falls back to memory when sessionStorage throws', () => {
      mockSessionStorage.setItem.mockImplementation(() => {
        throw new Error('QuotaExceededError');
      });
      tabStorage = new TabStorage();
      tabStorage.set('test', 'value');
      // Should still work (using memory)
      const result = tabStorage.get<string>('test');
      expect(result).toBe('value');
    });
  });

  describe('get/set behavior', () => {
    beforeEach(() => {
      tabStorage = new TabStorage();
    });

    it('returns parsed JSON', () => {
      mockSessionStorage._store['nf_test'] = JSON.stringify({ tab: 'data' });
      const result = tabStorage.get<{ tab: string }>('test');
      expect(result).toEqual({ tab: 'data' });
    });

    it('returns null for missing key', () => {
      const result = tabStorage.get('nonexistent');
      expect(result).toBeNull();
    });

    it('uses key prefix nf_', () => {
      tabStorage.get('tab_id');
      expect(mockSessionStorage.getItem).toHaveBeenCalledWith('nf_tab_id');
    });
  });
});

describe('STORAGE_KEYS', () => {
  it('contains all expected keys', () => {
    expect(STORAGE_KEYS).toEqual({
      SESSION: 'session',
      PENDING_QUEUE: 'pending',
      TAB_ID: 'tab_id',
      DIMENSIONS: 'dimensions',
      IDENTITY: 'identity',
        LEGACY_USER_ID: 'user_id',
    });
  });
});
