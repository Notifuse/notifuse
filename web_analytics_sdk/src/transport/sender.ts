/**
 * V3 Session Payload Transport
 * Handles sending session payloads to the server with offline support
 */

import type { SessionPayload, SendResult } from '../types/session-state';
import { Storage, STORAGE_KEYS } from '../storage/storage';

// Type declaration for fetchLater API (Chrome 121+)
declare global {
  var fetchLater:
    | ((
        url: string,
        init?: RequestInit & { activateAfter?: number }
      ) => { activated: boolean })
    | undefined;
}

interface QueuedPayload {
  /** `${session_id}:${seq}` — identifies the beat so it can be removed on ack. */
  id: string;
  payload: SessionPayload;
  queuedAt: number;
}

/** What a send outcome means for the queued copy of the payload. */
type SendOutcome = 'ok' | 'permanent' | 'retryable';

const MAX_QUEUE_SIZE = 100;
const QUEUE_TTL_MS = 24 * 60 * 60 * 1000; // 24 hours
const TIMEOUT_MS = 10000; // 10 seconds

export class Sender {
  private readonly endpoint: string;
  private readonly storage: Storage;
  private readonly debug: boolean;
  /**
   * The queue is namespaced per tab. It lives in localStorage — the only store
   * that is synchronous at pagehide, and the only one that survives the tab —
   * but localStorage is shared across tabs and updated by non-atomic
   * read-modify-write, so a single key means two tabs flushing at once erase
   * each other's re-queued items.
   */
  private readonly queueKey: string;
  private isFlushing: boolean = false;

  constructor(endpoint: string, storage: Storage, debug: boolean = false, tabId: number = 0) {
    this.endpoint = endpoint;
    this.storage = storage;
    this.debug = debug;
    this.queueKey = `${STORAGE_KEYS.PENDING_QUEUE}_${tabId}`;

    // Listen for online event to flush queue
    if (typeof window !== 'undefined') {
      window.addEventListener('online', () => this.handleOnline());
    }
  }

  /**
   * Stringify payload with sent_at timestamp injected at send time.
   * CRITICAL: Call this at every HTTP send point, not when building/caching payload.
   */
  private stringifyWithSentAt(payload: SessionPayload): string {
    return JSON.stringify({
      ...payload,
      sent_at: Date.now(),
    });
  }

  /**
   * Check if browser is offline
   */
  private isOffline(): boolean {
    return typeof navigator !== 'undefined' && navigator.onLine === false;
  }

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
  private classifyStatus(status: number): SendOutcome {
    if (status >= 200 && status < 300) return 'ok';
    if (status === 408 || status === 429) return 'retryable';
    if (status >= 400 && status < 500) return 'permanent';
    return 'retryable';
  }

  /**
   * Write the payload to durable storage before any send is attempted.
   *
   * Persist-then-send is the only ordering that survives the tab dying
   * mid-flight, and it is safe precisely because duplicates are free: the
   * server's `EXCLUDED.beat_seq > beat_seq` guard makes a replayed beat a
   * no-op, while a dropped one costs everything since the last success.
   */
  private persist(payload: SessionPayload): string {
    const id = `${payload.session_id}:${payload.seq}`;
    const queue = this.getQueue().filter((item) => item.id !== id);
    queue.push({ id, payload, queuedAt: Date.now() });
    this.saveQueue(queue);
    return id;
  }

  /** Remove one settled beat, leaving concurrently-added ones untouched. */
  private dequeue(id: string): void {
    this.saveQueue(this.getQueue().filter((item) => item.id !== id));
  }

  /**
   * One HTTP attempt, always bounded by a timeout — without one a single hung
   * connection stalls a drain indefinitely.
   */
  private async attempt(
    payload: SessionPayload
  ): Promise<{ outcome: SendOutcome; error?: string; data?: unknown }> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);
    try {
      const response = await fetch(`${this.endpoint}/track`, {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain;charset=UTF-8' },
        body: this.stringifyWithSentAt(payload), // Fresh sent_at at send time
        keepalive: true,
        signal: controller.signal,
      });

      if (response.ok) {
        const data = await Promise.resolve(response.json?.()).catch(() => undefined);
        return { outcome: 'ok', data };
      }

      const status = typeof response.status === 'number' ? response.status : 500;
      return {
        outcome: this.classifyStatus(status),
        error: `HTTP ${response.status}: ${response.statusText}`,
      };
    } catch (error) {
      // The abort is our own timeout firing, not a server signal; keep the
      // friendlier label callers already match on.
      const aborted = error instanceof Error && error.name === 'AbortError';
      return {
        outcome: 'retryable',
        error: aborted
          ? 'Request timeout'
          : error instanceof Error
            ? error.message
            : 'Unknown error',
      };
    } finally {
      clearTimeout(timeoutId);
    }
  }

  /**
   * Get pending queue from storage
   */
  private getQueue(): QueuedPayload[] {
    return this.storage.get<QueuedPayload[]>(this.queueKey) || [];
  }

  /**
   * Save queue to storage (with size limit)
   */
  private saveQueue(queue: QueuedPayload[]): void {
    const trimmed = queue.slice(-MAX_QUEUE_SIZE);
    this.storage.set(this.queueKey, trimmed);
  }

  /**
   * Drain the durable queue.
   *
   * Each item is removed only once its own send has settled in a way that means
   * it will never succeed again — a 2xx, or a permanent rejection. The previous
   * implementation blanked the whole queue before sending anything, so a tab
   * closed mid-drain took every un-sent item with it.
   */
  async flushQueue(): Promise<void> {
    if (this.isFlushing) return;
    this.isFlushing = true;

    try {
      const now = Date.now();
      for (const item of this.getQueue()) {
        if (now - item.queuedAt > QUEUE_TTL_MS) {
          this.dequeue(item.id);
          continue;
        }
        if (this.isOffline()) break;

        const result = await this.attempt(item.payload);
        if (result.outcome !== 'retryable') {
          this.dequeue(item.id);
        }
      }
    } finally {
      this.isFlushing = false;
    }
  }

  /**
   * Flush queue when back online.
   *
   * The `online` edge is only one of the triggers: a fresh page load can never
   * observe it, so the SDK also calls flushQueue() at init. Without that, the
   * commonest offline pattern — browse offline, close the tab, reconnect with
   * no page open — leaves the queue untouched until its TTL discards it.
   */
  async handleOnline(): Promise<void> {
    if (this.debug) {
      console.log('[NotifuseAnalytics] Back online, flushing queue');
    }
    await this.flushQueue();
  }

  /**
   * Send session payload via fetch.
   *
   * Retry eligibility comes from the outcome, never from navigator.onLine: that
   * is a link-layer signal and stays true behind a captive portal, a dead
   * upstream, a CSP block or an ad-blocker, which is most real-world failure.
   */
  async sendSession(payload: SessionPayload): Promise<SendResult> {
    const id = this.persist(payload);

    if (this.isOffline()) {
      return { success: false, error: 'offline', queued: true };
    }

    if (this.debug) {
      console.log('[NotifuseAnalytics] Sending session payload:', payload);
    }

    const result = await this.attempt(payload);

    if (result.outcome !== 'retryable') {
      this.dequeue(id);
    }
    if (result.outcome === 'ok') {
      if (this.debug) {
        console.log('[NotifuseAnalytics] Session response:', result.data);
      }
      return { success: true };
    }

    if (this.debug) {
      console.error('[NotifuseAnalytics] Send failed:', result.error);
    }
    return {
      success: false,
      error: result.error,
      queued: result.outcome === 'retryable',
    };
  }

  /**
   * Send session payload via sendBeacon (for unload)
   * IMPORTANT: sent_at is set fresh at each send attempt, not cached.
   */
  sendSessionBeacon(payload: SessionPayload): boolean {
    // The terminal beat is the one whose loss is unrecoverable, and none of the
    // transports below can confirm delivery. So it is persisted first and left
    // queued: the next page load replays it once, and the server's beat_seq
    // guard turns that replay into a no-op if it did arrive.
    this.persist(payload);

    if (this.isOffline()) {
      return false;
    }

    const url = `${this.endpoint}/track`;

    if (this.debug) {
      console.log('[NotifuseAnalytics] Sending session beacon:', payload);
    }

    // 1. Try fetchLater first (Chrome 121+, guaranteed delivery)
    if (typeof fetchLater === 'function') {
      try {
        fetchLater(url, {
          method: 'POST',
          headers: { 'Content-Type': 'text/plain;charset=UTF-8' },
          body: this.stringifyWithSentAt(payload), // Fresh sent_at
          activateAfter: 0,
        });
        if (this.debug) {
          console.log('[NotifuseAnalytics] Session queued via fetchLater');
        }
        return true;
      } catch {
        // Fall through to sendBeacon
      }
    }

    // Safari beacon limit is 64KB, but older versions had 16KB
    // Use 15KB threshold for safety
    const MAX_BEACON_SIZE = 15 * 1024;
    const bodyForBeacon = this.stringifyWithSentAt(payload); // Fresh sent_at
    const useBeacon = bodyForBeacon.length <= MAX_BEACON_SIZE;

    // 2. Try sendBeacon (if payload is small enough)
    if (useBeacon && navigator.sendBeacon) {
      try {
        const blob = new Blob([bodyForBeacon], { type: 'text/plain;charset=UTF-8' });
        const success = navigator.sendBeacon(url, blob);
        if (success) {
          if (this.debug) {
            console.log('[NotifuseAnalytics] Session sent via beacon');
          }
          return true;
        }
      } catch {
        // Fall through to fetch fallback
      }
    }

    // 3. Fallback to fetch with keepalive (also used for large payloads).
    // A keepalive body over the origin's 64KiB budget, a CSP violation or a
    // blocker all reject the promise rather than throwing synchronously, so the
    // catch below cannot see them — hence the explicit .catch(), which also
    // stops an unhandled rejection leaking into the customer's error tracking.
    try {
      const pending = fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'text/plain;charset=UTF-8',
        },
        body: this.stringifyWithSentAt(payload), // Fresh sent_at
        keepalive: true,
      });
      if (pending && typeof pending.catch === 'function') {
        pending.catch(() => {
          if (this.debug) {
            console.warn('[NotifuseAnalytics] keepalive fetch rejected; beat stays queued');
          }
        });
      }
      if (this.debug) {
        console.log('[NotifuseAnalytics] Session sent via fetch keepalive');
      }
      return true;
    } catch {
      return false;
    }
  }
}
