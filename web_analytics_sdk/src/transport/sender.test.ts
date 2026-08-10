/**
 * Tests for Sender
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { Sender } from './sender';
import { Storage } from '../storage/storage';
import type { SessionPayload } from '../types/session-state';

describe('Sender', () => {
  let sender: Sender;
  let mockStorage: Storage;
  let mockLocalStorage: ReturnType<typeof createMockStorage>;
  let fetchMock: ReturnType<typeof vi.fn>;
  let sendBeaconMock: ReturnType<typeof vi.fn>;

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

  const createPayload = (): SessionPayload => ({
    workspace_id: 'ws-123',
    session_id: 'session-123',
    tab_id: 7,
    seq: 1,
    actions: [
      {
        type: 'pageview',
        path: '/home',
        page_number: 1,
        duration: 5000,
        scroll: 50,
        entered_at: Date.now() - 5000,
        exited_at: Date.now(),
      },
    ],
    created_at: Date.now() - 60000,
    updated_at: Date.now(),
    sdk_version: '6.0.0',
  });

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-15T12:00:00.000Z'));

    mockLocalStorage = createMockStorage();
    vi.stubGlobal('localStorage', mockLocalStorage);

    fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ success: true, checkpoint: 0 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    sendBeaconMock = vi.fn().mockReturnValue(true);
    vi.stubGlobal('navigator', { sendBeacon: sendBeaconMock });

    mockStorage = new Storage();
    sender = new Sender('https://api.example.com', mockStorage);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  describe('sendSession', () => {
    it('sends payload via fetch to correct endpoint', async () => {
      const payload = createPayload();
      await sender.sendSession(payload);

      expect(fetchMock).toHaveBeenCalledWith(
        'https://api.example.com/track',
        expect.objectContaining({
          method: 'POST',
          headers: { 'Content-Type': 'text/plain;charset=UTF-8' },
          keepalive: true,
        })
      );
      // Verify body contains the payload fields plus sent_at
      const sentBody = JSON.parse(fetchMock.mock.calls[0][1].body);
      expect(sentBody.workspace_id).toBe(payload.workspace_id);
      expect(sentBody.session_id).toBe(payload.session_id);
      expect(sentBody.sent_at).toBeDefined();
    });

    it('returns success on successful response', async () => {
      fetchMock.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ success: true }),
      });

      const result = await sender.sendSession(createPayload());

      expect(result.success).toBe(true);
    });

    it('returns error on HTTP failure', async () => {
      fetchMock.mockResolvedValue({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
      });

      const result = await sender.sendSession(createPayload());

      expect(result.success).toBe(false);
      expect(result.error).toBe('HTTP 500: Internal Server Error');
    });

    it('returns error on network failure', async () => {
      fetchMock.mockRejectedValue(new Error('Network error'));

      const result = await sender.sendSession(createPayload());

      expect(result.success).toBe(false);
      expect(result.error).toBe('Network error');
    });

    it('returns error on unknown error', async () => {
      fetchMock.mockRejectedValue('unknown');

      const result = await sender.sendSession(createPayload());

      expect(result.success).toBe(false);
      expect(result.error).toBe('Unknown error');
    });
  });

  describe('sendSessionBeacon', () => {
    it('sends payload via sendBeacon', () => {
      const payload = createPayload();
      const result = sender.sendSessionBeacon(payload);

      expect(result).toBe(true);
      expect(sendBeaconMock).toHaveBeenCalledWith(
        'https://api.example.com/track',
        expect.any(Blob)
      );
    });

    it('creates Blob with text/plain type (keeps the POST a CORS simple request)', () => {
      const payload = createPayload();
      sender.sendSessionBeacon(payload);

      const blobArg = sendBeaconMock.mock.calls[0][1] as Blob;
      expect(blobArg.type).toBe('text/plain;charset=utf-8');
    });

    it('falls back to fetch when sendBeacon fails', () => {
      sendBeaconMock.mockReturnValue(false);

      const payload = createPayload();
      const result = sender.sendSessionBeacon(payload);

      expect(result).toBe(true);
      expect(fetchMock).toHaveBeenCalledWith(
        'https://api.example.com/track',
        expect.objectContaining({
          method: 'POST',
          keepalive: true,
        })
      );
    });

    it('falls back to fetch when sendBeacon throws', () => {
      sendBeaconMock.mockImplementation(() => {
        throw new Error('Beacon error');
      });

      const payload = createPayload();
      const result = sender.sendSessionBeacon(payload);

      expect(result).toBe(true);
      expect(fetchMock).toHaveBeenCalled();
    });

    it('returns false when both sendBeacon and fetch fail', () => {
      sendBeaconMock.mockReturnValue(false);
      fetchMock.mockImplementation(() => {
        throw new Error('Fetch error');
      });

      const payload = createPayload();
      const result = sender.sendSessionBeacon(payload);

      expect(result).toBe(false);
    });

    it('works when sendBeacon is not available', () => {
      vi.stubGlobal('navigator', { sendBeacon: undefined });

      const payload = createPayload();
      const result = sender.sendSessionBeacon(payload);

      expect(result).toBe(true);
      expect(fetchMock).toHaveBeenCalled();
    });

    it('skips sendBeacon for payloads > 15KB and uses fetch directly', () => {
      // Create a payload larger than 15KB
      const largePayload = createPayload();
      largePayload.attributes = {
        landing_page: 'https://example.com',
        user_agent: 'x'.repeat(16 * 1024), // 16KB string
      };

      const result = sender.sendSessionBeacon(largePayload);

      // Should NOT call sendBeacon for large payloads
      expect(sendBeaconMock).not.toHaveBeenCalled();
      // Should use fetch directly
      expect(fetchMock).toHaveBeenCalledWith(
        'https://api.example.com/track',
        expect.objectContaining({
          method: 'POST',
          keepalive: true,
        })
      );
      expect(result).toBe(true);
    });
  });

  describe('debug mode', () => {
    beforeEach(() => {
      vi.spyOn(console, 'log').mockImplementation(() => {});
      vi.spyOn(console, 'error').mockImplementation(() => {});
      sender = new Sender('https://api.example.com', mockStorage, true);
    });

    afterEach(() => {
      vi.restoreAllMocks();
    });

    it('logs sendSession payload in debug mode', async () => {
      await sender.sendSession(createPayload());

      expect(console.log).toHaveBeenCalledWith(
        '[NotifuseAnalytics] Sending session payload:',
        expect.any(Object)
      );
    });

    it('logs sendSession response in debug mode', async () => {
      fetchMock.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ success: true, checkpoint: 3 }),
      });

      await sender.sendSession(createPayload());

      expect(console.log).toHaveBeenCalledWith(
        '[NotifuseAnalytics] Session response:',
        { success: true, checkpoint: 3 }
      );
    });

    it('logs errors in debug mode', async () => {
      fetchMock.mockRejectedValue(new Error('Network error'));

      await sender.sendSession(createPayload());

      // Every failure path now reports a normalised message rather than the raw
      // Error, so one classifier can decide retryability for all of them.
      expect(console.error).toHaveBeenCalledWith(
        '[NotifuseAnalytics] Send failed:',
        expect.any(String)
      );
    });

    it('logs beacon send in debug mode', () => {
      sender.sendSessionBeacon(createPayload());

      expect(console.log).toHaveBeenCalledWith(
        '[NotifuseAnalytics] Sending session beacon:',
        expect.any(Object)
      );
      expect(console.log).toHaveBeenCalledWith(
        '[NotifuseAnalytics] Session sent via beacon'
      );
    });

    it('logs fetch fallback in debug mode', () => {
      sendBeaconMock.mockReturnValue(false);

      sender.sendSessionBeacon(createPayload());

      expect(console.log).toHaveBeenCalledWith(
        '[NotifuseAnalytics] Session sent via fetch keepalive'
      );
    });
  });

  describe('offline detection and queue', () => {
    describe('when offline', () => {
      beforeEach(() => {
        vi.stubGlobal('navigator', { onLine: false, sendBeacon: vi.fn() });
      });

      it('queues payload to storage when offline', async () => {
        const result = await sender.sendSession(createPayload());

        expect(result.success).toBe(false);
        expect(result.error).toBe('offline');
        expect(result.queued).toBe(true);
        expect(fetchMock).not.toHaveBeenCalled();

        // Verify queued in storage (Storage class adds 'nf_' prefix to 'pending')
        const queue = JSON.parse(mockLocalStorage._store['nf_pending_0'] || '[]');
        expect(queue.length).toBe(1);
      });

      it('sendSessionBeacon queues and returns false when offline', () => {
        const result = sender.sendSessionBeacon(createPayload());

        expect(result).toBe(false);

        const queue = JSON.parse(mockLocalStorage._store['nf_pending_0'] || '[]');
        expect(queue.length).toBe(1);
      });
    });

    describe('when back online', () => {
      it('flushes queue when online event fires', async () => {
        // Queue a payload while offline
        vi.stubGlobal('navigator', { onLine: false, sendBeacon: vi.fn() });
        await sender.sendSession(createPayload());

        // Come back online
        vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });

        // Simulate online event
        await sender.handleOnline();

        expect(fetchMock).toHaveBeenCalled();

        // Queue should be empty
        const queue = JSON.parse(mockLocalStorage._store['nf_pending_0'] || '[]');
        expect(queue.length).toBe(0);
      });

      it('removes successfully sent items from queue', async () => {
        // Queue multiple payloads
        vi.stubGlobal('navigator', { onLine: false, sendBeacon: vi.fn() });
        // Two distinct beats: the queue is keyed by session_id:seq, so persisting
        // the same beat twice deliberately collapses to one entry.
        await sender.sendSession({ ...createPayload(), seq: 1 });
        await sender.sendSession({ ...createPayload(), seq: 2 });

        let queue = JSON.parse(mockLocalStorage._store['nf_pending_0'] || '[]');
        expect(queue.length).toBe(2);

        // Come back online
        vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });
        await sender.handleOnline();

        queue = JSON.parse(mockLocalStorage._store['nf_pending_0'] || '[]');
        expect(queue.length).toBe(0);
      });

      it('keeps failed items in queue for retry', async () => {
        vi.stubGlobal('navigator', { onLine: false, sendBeacon: vi.fn() });
        await sender.sendSession(createPayload());

        // Come online but server fails
        vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });
        fetchMock.mockResolvedValueOnce({ ok: false, status: 500, statusText: 'Error' });

        await sender.handleOnline();

        // Should still be in queue
        const queue = JSON.parse(mockLocalStorage._store['nf_pending_0'] || '[]');
        expect(queue.length).toBe(1);
      });
    });

    describe('queue management', () => {
      it('limits queue size to prevent storage bloat', async () => {
        vi.stubGlobal('navigator', { onLine: false, sendBeacon: vi.fn() });

        // Queue 100+ payloads
        for (let i = 0; i < 110; i++) {
          await sender.sendSession(createPayload());
        }

        const queue = JSON.parse(mockLocalStorage._store['nf_pending_0'] || '[]');
        expect(queue.length).toBeLessThanOrEqual(100); // Max 100 items
      });

      it('expires old queue items after 24 hours', async () => {
        vi.stubGlobal('navigator', { onLine: false, sendBeacon: vi.fn() });

        // Queue payload
        await sender.sendSession(createPayload());

        // Advance 25 hours
        vi.advanceTimersByTime(25 * 60 * 60 * 1000);

        // Come online
        vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });
        await sender.handleOnline();

        // Expired items should be discarded, not sent
        expect(fetchMock).not.toHaveBeenCalled();

        // Queue should be empty (expired item discarded)
        const queue = JSON.parse(mockLocalStorage._store['nf_pending_0'] || '[]');
        expect(queue.length).toBe(0);
      });
    });
  });

  describe('request timeout', () => {
    it('aborts fetch after 10 seconds', async () => {
      vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });
      // Mock fetch that respects abort signal
      fetchMock.mockImplementation((_url: string, options?: RequestInit) => {
        return new Promise((_resolve, reject) => {
          if (options?.signal) {
            options.signal.addEventListener('abort', () => {
              const error = new Error('The operation was aborted');
              error.name = 'AbortError';
              reject(error);
            });
          }
        });
      });

      const sendPromise = sender.sendSession(createPayload());

      await vi.advanceTimersByTimeAsync(10001);

      const result = await sendPromise;

      expect(result.success).toBe(false);
      expect(result.error).toBe('Request timeout');
    });

    it('clears timeout on successful response', async () => {
      vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });

      const result = await sender.sendSession(createPayload());

      expect(result.success).toBe(true);
    });

    it('clears timeout on network error', async () => {
      vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });
      fetchMock.mockRejectedValue(new Error('Network error'));

      const result = await sender.sendSession(createPayload());

      expect(result.success).toBe(false);
      expect(result.error).toBe('Network error');
    });

    it('passes AbortSignal to fetch', async () => {
      vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });

      await sender.sendSession(createPayload());

      expect(fetchMock).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          signal: expect.any(AbortSignal),
        })
      );
    });

    it('queues payload on timeout for retry', async () => {
      vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });
      // Mock fetch that respects abort signal
      fetchMock.mockImplementation((_url: string, options?: RequestInit) => {
        return new Promise((_resolve, reject) => {
          if (options?.signal) {
            options.signal.addEventListener('abort', () => {
              const error = new Error('The operation was aborted');
              error.name = 'AbortError';
              reject(error);
            });
          }
        });
      });

      const sendPromise = sender.sendSession(createPayload());
      await vi.advanceTimersByTimeAsync(10001);
      const result = await sendPromise;

      expect(result.queued).toBe(true);

      const queue = JSON.parse(mockLocalStorage._store['nf_pending_0'] || '[]');
      expect(queue.length).toBe(1);
    });
  });

  describe('sent_at injection', () => {
    it('adds sent_at to payload when sending via fetch', async () => {
      const payload = createPayload();
      await sender.sendSession(payload);

      const sentBody = JSON.parse(fetchMock.mock.calls[0][1].body);
      expect(sentBody.sent_at).toBeDefined();
      expect(typeof sentBody.sent_at).toBe('number');
    });

    it('sets sent_at to current timestamp at send time', async () => {
      const beforeTime = Date.now();
      const payload = createPayload();
      await sender.sendSession(payload);

      const sentBody = JSON.parse(fetchMock.mock.calls[0][1].body);
      expect(sentBody.sent_at).toBeGreaterThanOrEqual(beforeTime);
      expect(sentBody.sent_at).toBeLessThanOrEqual(Date.now());
    });

    it('does not mutate original payload', async () => {
      const payload = createPayload();
      const originalPayload = { ...payload };
      await sender.sendSession(payload);

      expect(payload).toEqual(originalPayload);
      expect(payload).not.toHaveProperty('sent_at');
    });

    it('adds sent_at to beacon payload', () => {
      const payload = createPayload();
      sender.sendSessionBeacon(payload);

      // Check the blob content
      const blobArg = sendBeaconMock.mock.calls[0][1] as Blob;
      expect(blobArg.type).toBe('text/plain;charset=utf-8');
      // Note: Blob content is checked via the reader pattern in integration tests
    });

    it('generates fresh sent_at for each send attempt', async () => {
      const payload = createPayload();

      // First send
      await sender.sendSession(payload);
      const firstSentAt = JSON.parse(fetchMock.mock.calls[0][1].body).sent_at;

      // Advance time
      vi.advanceTimersByTime(1000);

      // Second send (same payload)
      await sender.sendSession(payload);
      const secondSentAt = JSON.parse(fetchMock.mock.calls[1][1].body).sent_at;

      expect(secondSentAt).toBeGreaterThan(firstSentAt);
    });

    it('adds sent_at when flushing offline queue', async () => {
      // Queue a payload while offline
      vi.stubGlobal('navigator', { onLine: false, sendBeacon: vi.fn() });
      await sender.sendSession(createPayload());

      // Advance time significantly
      vi.advanceTimersByTime(60000);

      // Come back online
      vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });
      await sender.handleOnline();

      // sent_at should be set at flush time, not queue time
      const sentBody = JSON.parse(fetchMock.mock.calls[0][1].body);
      expect(sentBody.sent_at).toBeDefined();
      // sent_at should reflect when it was actually sent (after advancing time)
      expect(sentBody.sent_at).toBeGreaterThan(Date.now() - 5000);
    });
  });

  describe('per-tab queue isolation (W0.1)', () => {
    it('two tabs do not share one queue key', async () => {
      // The queue lived under a single origin-wide key mutated by non-atomic
      // read-modify-write from every tab. Two tabs regaining connectivity both
      // read it and both wrote it back, so the second one's write erased items
      // the first had just re-queued after a failure.
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));

      const tabA = new Sender('https://api.example.com', mockStorage, false, 111);
      const tabB = new Sender('https://api.example.com', mockStorage, false, 222);
      await tabA.sendSession({ ...createPayload(), session_id: 's', seq: 1 });
      await tabB.sendSession({ ...createPayload(), session_id: 's', seq: 1 });

      expect(JSON.parse(mockLocalStorage._store['nf_pending_111'])).toHaveLength(1);
      expect(JSON.parse(mockLocalStorage._store['nf_pending_222'])).toHaveLength(1);
    });

    it('a drain in one tab leaves the other tab backlog intact', async () => {
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));
      const tabA = new Sender('https://api.example.com', mockStorage, false, 111);
      const tabB = new Sender('https://api.example.com', mockStorage, false, 222);
      await tabA.sendSession(createPayload());
      await tabB.sendSession(createPayload());

      fetchMock.mockResolvedValue({ ok: true, json: () => Promise.resolve({ success: true }) });
      await tabA.flushQueue();

      expect(JSON.parse(mockLocalStorage._store['nf_pending_111'])).toHaveLength(0);
      expect(JSON.parse(mockLocalStorage._store['nf_pending_222'])).toHaveLength(1);
    });
  });

  describe('delivery outcomes (W0.5)', () => {
    const queue = () => JSON.parse(mockLocalStorage._store['nf_pending_0'] ?? '[]');

    it('enqueues a rejected fetch even though navigator reports online', () => {
      // navigator.onLine is a link-layer signal: it stays true behind a captive
      // portal, a dead upstream, a CSP block or an ad-blocker — which is most
      // real-world failure. Keying retry on it means almost nothing is ever
      // retried, and the last beat of a session is lost outright.
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));
      return sender.sendSession(createPayload()).then((result) => {
        expect(result.success).toBe(false);
        expect(queue()).toHaveLength(1);
      });
    });

    it('keeps the payload queued on a 5xx and drops it on a 2xx', async () => {
      fetchMock.mockResolvedValue({ ok: false, status: 503, statusText: 'Service Unavailable' });
      await sender.sendSession(createPayload());
      expect(queue()).toHaveLength(1);

      fetchMock.mockResolvedValue({ ok: true, json: () => Promise.resolve({ success: true }) });
      await sender.sendSession(createPayload());
      expect(queue()).toHaveLength(0);
    });

    it('drops a 400 without ever retrying it', async () => {
      // The server answers 200 for everything it wants un-retried, so a 400 means
      // this exact payload can never succeed. Retrying it forever poisons the
      // queue and evicts payloads that could still land.
      fetchMock.mockResolvedValue({ ok: false, status: 400, statusText: 'Bad Request' });
      await sender.sendSession(createPayload());
      expect(queue()).toHaveLength(0);
    });

    it('persists the payload before the request settles', async () => {
      // Persist-then-send is the only ordering that survives the tab dying
      // mid-flight. Duplicates are free under the beat_seq guard; drops are not.
      let queuedDuringFlight = -1;
      fetchMock.mockImplementation(() => {
        queuedDuringFlight = queue().length;
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ success: true }) });
      });

      await sender.sendSession(createPayload());
      expect(queuedDuringFlight).toBe(1);
      expect(queue()).toHaveLength(0);
    });

    it('a drain that fails partway leaves the unsent items queued', async () => {
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));
      for (const seq of [1, 2, 3]) {
        await sender.sendSession({ ...createPayload(), seq });
      }
      expect(queue()).toHaveLength(3);

      let call = 0;
      fetchMock.mockImplementation(() => {
        call += 1;
        return call === 1
          ? Promise.resolve({ ok: true, json: () => Promise.resolve({ success: true }) })
          : Promise.reject(new TypeError('Failed to fetch'));
      });

      await sender.flushQueue();
      expect(queue()).toHaveLength(2);
    });

    it('flushQueue drains without an online event ever firing', async () => {
      // The only drain trigger today is window's `online` edge, which a fresh
      // page load can never observe — so the commonest offline pattern leaves
      // the queue untouched until its TTL silently discards it.
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));
      await sender.sendSession(createPayload());
      expect(queue()).toHaveLength(1);

      fetchMock.mockResolvedValue({ ok: true, json: () => Promise.resolve({ success: true }) });
      await sender.flushQueue();
      expect(queue()).toHaveLength(0);
    });

    it('leaves the terminal beat queued so the next load can replay it', () => {
      // No transport on the unload path can confirm delivery — sendBeacon returns
      // only "queued by the UA", fetchLater returns before anything is sent, and
      // the keepalive fetch rejects asynchronously. The guarantee therefore lives
      // in the queue, not in the return value: one redundant request on the next
      // page load, which the beat_seq guard no-ops, buys back the one beat whose
      // loss is unrecoverable.
      sendBeaconMock.mockReturnValue(false);
      sender.sendSessionBeacon(createPayload());
      expect(queue()).toHaveLength(1);
    });
  });

  describe('fetchLater progressive enhancement', () => {
    it('uses fetchLater when available', () => {
      const fetchLaterMock = vi.fn().mockReturnValue({ activated: false });
      vi.stubGlobal('fetchLater', fetchLaterMock);
      vi.stubGlobal('navigator', { onLine: true, sendBeacon: sendBeaconMock });

      const result = sender.sendSessionBeacon(createPayload());

      expect(result).toBe(true);
      expect(fetchLaterMock).toHaveBeenCalledWith(
        'https://api.example.com/track',
        expect.objectContaining({
          method: 'POST',
          body: expect.any(String),
          activateAfter: 0,
        })
      );
      expect(sendBeaconMock).not.toHaveBeenCalled();
    });

    it('falls back to sendBeacon when fetchLater unavailable', () => {
      vi.stubGlobal('fetchLater', undefined);
      vi.stubGlobal('navigator', { onLine: true, sendBeacon: sendBeaconMock });

      sender.sendSessionBeacon(createPayload());

      expect(sendBeaconMock).toHaveBeenCalled();
    });

    it('falls back to sendBeacon when fetchLater throws', () => {
      vi.stubGlobal('fetchLater', vi.fn().mockImplementation(() => {
        throw new Error('fetchLater error');
      }));
      vi.stubGlobal('navigator', { onLine: true, sendBeacon: sendBeaconMock });

      const result = sender.sendSessionBeacon(createPayload());

      expect(result).toBe(true);
      expect(sendBeaconMock).toHaveBeenCalled();
    });

    it('logs fetchLater usage in debug mode', () => {
      const consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
      const fetchLaterMock = vi.fn().mockReturnValue({ activated: false });
      vi.stubGlobal('fetchLater', fetchLaterMock);
      vi.stubGlobal('navigator', { onLine: true, sendBeacon: vi.fn() });

      const debugSender = new Sender('https://api.example.com', mockStorage, true);
      debugSender.sendSessionBeacon(createPayload());

      expect(consoleSpy).toHaveBeenCalledWith(
        '[NotifuseAnalytics] Session queued via fetchLater'
      );

      consoleSpy.mockRestore();
    });
  });
});
