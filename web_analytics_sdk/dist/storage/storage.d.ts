/**
 * Storage module with localStorage + memory fallback
 * Handles Safari Private Mode gracefully
 */
export declare class Storage {
    private useMemory;
    private memory;
    constructor();
    /**
     * Test localStorage availability
     * Safari Private Mode throws QuotaExceededError even on empty storage
     */
    private testStorage;
    /**
     * Get a value from storage
     */
    get<T>(key: string): T | null;
    /**
     * Set a value in storage
     */
    set<T>(key: string, value: T): void;
    /**
     * Remove a value from storage
     */
    remove(key: string): void;
    /**
     * Clear all SDK storage
     */
    clear(): void;
    /**
     * Check if using memory fallback
     */
    isUsingMemory(): boolean;
}
export declare class TabStorage {
    private memory;
    private useMemory;
    constructor();
    private testStorage;
    get<T>(key: string): T | null;
    set<T>(key: string, value: T): void;
}
export declare const STORAGE_KEYS: {
    readonly SESSION: "session";
    readonly PENDING_QUEUE: "pending";
    readonly TAB_ID: "tab_id";
    readonly DIMENSIONS: "dimensions";
    readonly IDENTITY: "identity";
    /** Written by builds before verified identity; purged on init. */
    readonly LEGACY_USER_ID: "user_id";
};
