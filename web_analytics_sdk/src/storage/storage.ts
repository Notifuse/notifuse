/**
 * Storage module with localStorage + memory fallback
 * Handles Safari Private Mode gracefully
 */

const STORAGE_PREFIX = 'nf_';

export class Storage {
  private useMemory = false;
  private memory = new Map<string, string>();

  constructor() {
    this.testStorage();
  }

  /**
   * Test localStorage availability
   * Safari Private Mode throws QuotaExceededError even on empty storage
   */
  private testStorage(): void {
    try {
      const testKey = STORAGE_PREFIX + 'test';
      localStorage.setItem(testKey, 'test');
      localStorage.removeItem(testKey);
    } catch {
      this.useMemory = true;
    }
  }

  /**
   * Get a value from storage
   */
  get<T>(key: string): T | null {
    const fullKey = STORAGE_PREFIX + key;

    if (this.useMemory) {
      const value = this.memory.get(fullKey);
      if (value === undefined) return null;
      try {
        return JSON.parse(value) as T;
      } catch {
        return null;
      }
    }

    try {
      const value = localStorage.getItem(fullKey);
      if (value === null) return null;
      return JSON.parse(value) as T;
    } catch {
      // Fallback to memory if localStorage fails mid-session
      this.useMemory = true;
      return this.memory.get(fullKey) ? JSON.parse(this.memory.get(fullKey)!) : null;
    }
  }

  /**
   * Set a value in storage
   */
  set<T>(key: string, value: T): void {
    const fullKey = STORAGE_PREFIX + key;
    const data = JSON.stringify(value);

    if (this.useMemory) {
      this.memory.set(fullKey, data);
      return;
    }

    try {
      localStorage.setItem(fullKey, data);
    } catch {
      // Quota exceeded mid-session - switch to memory
      this.useMemory = true;
      this.memory.set(fullKey, data);
    }
  }

  /**
   * Remove a value from storage
   */
  remove(key: string): void {
    const fullKey = STORAGE_PREFIX + key;

    if (this.useMemory) {
      this.memory.delete(fullKey);
      return;
    }

    try {
      localStorage.removeItem(fullKey);
    } catch {
      this.memory.delete(fullKey);
    }
  }

  /**
   * Clear all SDK storage
   */
  clear(): void {
    if (this.useMemory) {
      this.memory.clear();
      return;
    }

    try {
      // Only remove nf_ prefixed keys
      const keysToRemove: string[] = [];
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        if (key?.startsWith(STORAGE_PREFIX)) {
          keysToRemove.push(key);
        }
      }
      keysToRemove.forEach(key => localStorage.removeItem(key));
    } catch {
      this.memory.clear();
    }
  }

  /**
   * Check if using memory fallback
   */
  isUsingMemory(): boolean {
    return this.useMemory;
  }
}

// Session storage for tab-specific data
export class TabStorage {
  private memory = new Map<string, string>();
  private useMemory = false;

  constructor() {
    this.testStorage();
  }

  private testStorage(): void {
    try {
      const testKey = STORAGE_PREFIX + 'test';
      sessionStorage.setItem(testKey, 'test');
      sessionStorage.removeItem(testKey);
    } catch {
      this.useMemory = true;
    }
  }

  get<T>(key: string): T | null {
    const fullKey = STORAGE_PREFIX + key;

    if (this.useMemory) {
      const value = this.memory.get(fullKey);
      if (value === undefined) return null;
      try {
        return JSON.parse(value) as T;
      } catch {
        return null;
      }
    }

    try {
      const value = sessionStorage.getItem(fullKey);
      if (value === null) return null;
      return JSON.parse(value) as T;
    } catch {
      this.useMemory = true;
      return null;
    }
  }

  set<T>(key: string, value: T): void {
    const fullKey = STORAGE_PREFIX + key;
    const data = JSON.stringify(value);

    if (this.useMemory) {
      this.memory.set(fullKey, data);
      return;
    }

    try {
      sessionStorage.setItem(fullKey, data);
    } catch {
      this.useMemory = true;
      this.memory.set(fullKey, data);
    }
  }
}

// Storage keys
export const STORAGE_KEYS = {
  SESSION: 'session',
  PENDING_QUEUE: 'pending',
  TAB_ID: 'tab_id',
  DIMENSIONS: 'dimensions',
  IDENTITY: 'identity',
  /** Written by builds before verified identity; purged on init. */
  LEGACY_USER_ID: 'user_id',
} as const;
