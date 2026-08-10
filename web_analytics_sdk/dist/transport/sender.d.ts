/**
 * V3 Session Payload Transport
 * Handles sending session payloads to the server with offline support
 */
import type { SessionPayload, SendResult } from '../types/session-state';
import { Storage } from '../storage/storage';
declare global {
    var fetchLater: ((url: string, init?: RequestInit & {
        activateAfter?: number;
    }) => {
        activated: boolean;
    }) | undefined;
}
export declare class Sender {
    private readonly endpoint;
    private readonly storage;
    private readonly debug;
    private isFlushing;
    constructor(endpoint: string, storage: Storage, debug?: boolean);
    /**
     * Stringify payload with sent_at timestamp injected at send time.
     * CRITICAL: Call this at every HTTP send point, not when building/caching payload.
     */
    private stringifyWithSentAt;
    /**
     * Check if browser is offline
     */
    private isOffline;
    /**
     * Get pending queue from storage
     */
    private getQueue;
    /**
     * Save queue to storage (with size limit)
     */
    private saveQueue;
    /**
     * Add payload to offline queue
     */
    private enqueue;
    /**
     * Flush queue when back online
     */
    handleOnline(): Promise<void>;
    /**
     * Internal send without offline queue logic (for flush)
     */
    private sendSessionDirect;
    /**
     * Send session payload via fetch
     */
    sendSession(payload: SessionPayload): Promise<SendResult>;
    /**
     * Send session payload via sendBeacon (for unload)
     * IMPORTANT: sent_at is set fresh at each send attempt, not cached.
     */
    sendSessionBeacon(payload: SessionPayload): boolean;
}
