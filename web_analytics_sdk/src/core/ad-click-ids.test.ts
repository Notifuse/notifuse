import { describe, it, expect, beforeEach, vi } from 'vitest';
import { NotifuseAnalyticsSDK } from '../sdk';
import { DEFAULT_AD_CLICK_IDS } from '../utils/utm';

/**
 * adClickIds REPLACES the recognised list; extraAdClickIds ADDS to it.
 *
 * Two options rather than one because they answer different questions, and one
 * option cannot answer both: `[]` has to keep meaning "capture nothing", and a
 * site that wants one extra id cannot spell the additive case as a replacement
 * without pasting a copy of the whole default list — which then silently goes
 * stale as ids are added upstream. DEFAULT_AD_CLICK_IDS is not reachable from a
 * script tag either: the config object is authored before the bundle loads.
 */
describe('ad click id configuration', () => {
  beforeEach(() => {
    vi.resetModules();
    localStorage.clear();
    sessionStorage.clear();
  });

  const init = async (extra: Record<string, unknown>) => {
    const sdk = new NotifuseAnalyticsSDK();
    await sdk.init({
      workspace_id: 'ws_test',
      endpoint: 'https://api.example.com',
      ...extra,
    });
    return (await sdk.getConfig())!;
  };

  it('defaults to the built-in list', async () => {
    const config = await init({});
    expect(config.adClickIds).toEqual([...DEFAULT_AD_CLICK_IDS]);
  });

  it('extraAdClickIds appends, keeping the defaults', async () => {
    const config = await init({ extraAdClickIds: ['myclid'] });
    expect(config.adClickIds).toEqual([...DEFAULT_AD_CLICK_IDS, 'myclid']);
  });

  it('adClickIds still replaces the defaults entirely', async () => {
    const config = await init({ adClickIds: ['onlyclid'] });
    expect(config.adClickIds).toEqual(['onlyclid']);
  });

  it('adClickIds: [] still disables capture', async () => {
    // The reason extraAdClickIds had to be a second option rather than a change
    // of meaning for this one.
    const config = await init({ adClickIds: [] });
    expect(config.adClickIds).toEqual([]);
  });

  it('extraAdClickIds extends a replacement, not the defaults', async () => {
    // Precedence pinned explicitly rather than left to be discovered: extra is
    // applied to whichever list won the merge.
    const config = await init({ adClickIds: ['onlyclid'], extraAdClickIds: ['myclid'] });
    expect(config.adClickIds).toEqual(['onlyclid', 'myclid']);
  });

  it('extraAdClickIds can re-enable capture after adClickIds: []', async () => {
    const config = await init({ adClickIds: [], extraAdClickIds: ['myclid'] });
    expect(config.adClickIds).toEqual(['myclid']);
  });

  it('does not duplicate an id that is already recognised', async () => {
    const config = await init({ extraAdClickIds: ['gclid'] });
    expect(config.adClickIds).toEqual([...DEFAULT_AD_CLICK_IDS]);
  });

  it('treats a differently-cased duplicate as the same id', async () => {
    // Lookup is case-insensitive, so adding 'GCLID' would create a second entry
    // that can never be reached — the first match always wins.
    const config = await init({ extraAdClickIds: ['GCLID'] });
    expect(config.adClickIds).toEqual([...DEFAULT_AD_CLICK_IDS]);
  });

  it('appends in the order given, after the existing list', async () => {
    // Priority is list order, so a custom id must not outrank gclid by accident.
    const config = await init({ extraAdClickIds: ['aclid', 'bclid'] });
    const ids = config.adClickIds ?? [];
    expect(ids.slice(-2)).toEqual(['aclid', 'bclid']);
    expect(ids[0]).toBe('gclid');
  });
});
