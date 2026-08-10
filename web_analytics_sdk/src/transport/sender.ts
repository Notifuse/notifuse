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
  payload: SessionPayload;
  queuedAt: number;
}

const MAX_QUEUE_SIZE = 100;
const QUEUE_TTL_MS = 24 * 60 * 60 * 1000; // 24 hours
const TIMEOUT_MS = 10000; // 10 seconds

export class Sender {
  private readonly endpoint: string;
  private readonly storage: Storage;
  private readonly debug: boolean;
  private isFlushing: boolean = false;

  constructor(endpoint: string, storage: Storage, debug: boolean = false) {
    this.endpoint = endpoint;
    this.storage = storage;
    this.debug = debug;

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
   * Get pending queue from storage
   */
  private getQueue(): QueuedPayload[] {
    return this.storage.get<QueuedPayload[]>(STORAGE_KEYS.PENDING_QUEUE) || [];
  }

  /**
   * Save queue to storage (with size limit)
   */
  private saveQueue(queue: QueuedPayload[]): void {
    const trimmed = queue.slice(-MAX_QUEUE_SIZE);
    this.storage.set(STORAGE_KEYS.PENDING_QUEUE, trimmed);
  }

  /**
   * Add payload to offline queue
   */
  private enqueue(payload: SessionPayload): void {
    const queue = this.getQueue();
    queue.push({ payload, queuedAt: Date.now() });
    this.saveQueue(queue);

    if (this.debug) {
      console.log('[NotifuseAnalytics] Payload queued for later (offline)');
    }
  }

  /**
   * Flush queue when back online
   */
  async handleOnline(): Promise<void> {
    if (this.isFlushing) return;
    this.isFlushing = true;

    if (this.debug) {
      console.log('[NotifuseAnalytics] Back online, flushing queue');
    }

    try {
      const queue = this.getQueue();
      if (queue.length === 0) return;

      // Clear queue immediately to prevent duplicates
      this.storage.set(STORAGE_KEYS.PENDING_QUEUE, []);

      const now = Date.now();
      const failedItems: QueuedPayload[] = [];
      let expiredCount = 0;

      for (const item of queue) {
        // Skip expired items
        if (now - item.queuedAt > QUEUE_TTL_MS) {
          expiredCount++;
          continue;
        }

        // Send directly without re-queuing
        const result = await this.sendSessionDirect(item.payload);
        if (!result.success) {
          failedItems.push(item);
        }
      }

      if (expiredCount > 0 && this.debug) {
        console.log(`[NotifuseAnalytics] Discarded ${expiredCount} expired queue items`);
      }

      // Merge failed items back with any new items added during flush
      if (failedItems.length > 0) {
        const currentQueue = this.getQueue();
        this.saveQueue([...failedItems, ...currentQueue]);
      }
    } finally {
      this.isFlushing = false;
    }
  }

  /**
   * Internal send without offline queue logic (for flush)
   */
  private async sendSessionDirect(payload: SessionPayload): Promise<SendResult> {
    const url = `${this.endpoint}/track`;

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain;charset=UTF-8' },
        body: this.stringifyWithSentAt(payload), // Fresh sent_at at send time
        keepalive: true,
      });

      if (!response.ok) {
        return {
          success: false,
          error: `HTTP ${response.status}: ${response.statusText}`,
        };
      }

      // V3: Server returns success, no checkpoint needed
      await response.json();
      return { success: true };
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      };
    }
  }

  /**
   * Send session payload via fetch
   */
  async sendSession(payload: SessionPayload): Promise<SendResult> {
    // Queue if offline
    if (this.isOffline()) {
      this.enqueue(payload);
      return { success: false, error: 'offline', queued: true };
    }

    const url = `${this.endpoint}/track`;
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);

    if (this.debug) {
      console.log('[NotifuseAnalytics] Sending session payload:', payload);
    }

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'text/plain;charset=UTF-8',
        },
        body: this.stringifyWithSentAt(payload), // Fresh sent_at at send time
        keepalive: true,
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        return {
          success: false,
          error: `HTTP ${response.status}: ${response.statusText}`,
        };
      }

      // V3: Server returns success, no checkpoint needed
      const data = await response.json();

      if (this.debug) {
        console.log('[NotifuseAnalytics] Session response:', data);
      }

      return { success: true };
    } catch (error) {
      clearTimeout(timeoutId);

      if (this.debug) {
        console.error('[NotifuseAnalytics] Send failed:', error);
      }

      // Queue on timeout for retry
      if (error instanceof Error && error.name === 'AbortError') {
        this.enqueue(payload);
        return { success: false, error: 'Request timeout', queued: true };
      }

      return {
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      };
    }
  }

  /**
   * Send session payload via sendBeacon (for unload)
   * IMPORTANT: sent_at is set fresh at each send attempt, not cached.
   */
  sendSessionBeacon(payload: SessionPayload): boolean {
    // Queue if offline
    if (this.isOffline()) {
      this.enqueue(payload);
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

    // 3. Fallback to fetch with keepalive (also used for large payloads)
    try {
      fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'text/plain;charset=UTF-8',
        },
        body: this.stringifyWithSentAt(payload), // Fresh sent_at
        keepalive: true,
      });
      if (this.debug) {
        console.log('[NotifuseAnalytics] Session sent via fetch keepalive');
      }
      return true;
    } catch {
      return false;
    }
  }
}
