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
    /**
     * The queue is namespaced per tab. It lives in localStorage — the only store
     * that is synchronous at pagehide, and the only one that survives the tab —
     * but localStorage is shared across tabs and updated by non-atomic
     * read-modify-write, so a single key means two tabs flushing at once erase
     * each other's re-queued items.
     */
    private readonly queueKey;
    private isFlushing;
    constructor(endpoint: string, storage: Storage, debug?: boolean, tabId?: number);
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
     * Classify a failed send by whether retrying this exact payload could ever
     * succeed.
     *
     * The server answers 200 for everything it wants the client NOT to retry —
     * unknown workspace, feature disabled, disallowed domain, bot user-agent — so
     * a 4xx means the payload itself is unacceptable and will stay unacceptable;
     * retrying it forever only poisons the queue. 408 and 429 are the exceptions:
     * they are about timing, not content. 5xx and network failures are transient
     * by definition, and in a cumulative-snapshot model the retry is one
     * idempotent re-POST that supersedes everything before it.
     */
    private classifyStatus;
    /**
     * Write the payload to durable storage before any send is attempted.
     *
     * Persist-then-send is the only ordering that survives the tab dying
     * mid-flight, and it is safe precisely because duplicates are free: the
     * server's `EXCLUDED.beat_seq > beat_seq` guard makes a replayed beat a
     * no-op, while a dropped one costs everything since the last success.
     */
    private persist;
    /** Remove one settled beat, leaving concurrently-added ones untouched. */
    private dequeue;
    /**
     * One HTTP attempt, always bounded by a timeout — without one a single hung
     * connection stalls a drain indefinitely.
     */
    private attempt;
    /**
     * Get pending queue from storage
     */
    private getQueue;
    /**
     * Save queue to storage (with size limit)
     */
    private saveQueue;
    /**
     * Drain the durable queue.
     *
     * Each item is removed only once its own send has settled in a way that means
     * it will never succeed again — a 2xx, or a permanent rejection. The previous
     * implementation blanked the whole queue before sending anything, so a tab
     * closed mid-drain took every un-sent item with it.
     */
    flushQueue(): Promise<void>;
    /**
     * Flush queue when back online.
     *
     * The `online` edge is only one of the triggers: a fresh page load can never
     * observe it, so the SDK also calls flushQueue() at init. Without that, the
     * commonest offline pattern — browse offline, close the tab, reconnect with
     * no page open — leaves the queue untouched until its TTL discards it.
     */
    handleOnline(): Promise<void>;
    /**
     * Send session payload via fetch.
     *
     * Retry eligibility comes from the outcome, never from navigator.onLine: that
     * is a link-layer signal and stays true behind a captive portal, a dead
     * upstream, a CSP block or an ad-blocker, which is most real-world failure.
     */
    sendSession(payload: SessionPayload): Promise<SendResult>;
    /**
     * Send session payload via sendBeacon (for unload)
     * IMPORTANT: sent_at is set fresh at each send attempt, not cached.
     */
    sendSessionBeacon(payload: SessionPayload): boolean;
}
