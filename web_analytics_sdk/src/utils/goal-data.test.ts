import { describe, it, expect, beforeEach, afterEach, vi, type Mock } from 'vitest';
import { NotifuseAnalyticsSDK } from '../sdk';
import type { GoalData } from '../types';
import { MAX_GOAL_VALUE, normalizeGoalProperties, normalizeGoalValue } from './goal-data';

/**
 * trackGoal is called from plain JS far more than from TypeScript, so its
 * value and properties arrive with whatever types the page had at hand. The
 * server stores a number and a flat string map; a numeric property or a string
 * value used to reach it as-is, fail the whole beat's decode, and — because
 * actions[] is cumulative and a 4xx is permanent — lose every later beat of
 * the session. The SDK must never put a wrong type on the wire.
 */
describe('normalizeGoalValue', () => {
  it.each([
    [49.9, 49.9],
    [0, 0],
    ['49.90', 49.9],
    [' 12 ', 12],
    ['1e3', 1000],
  ])('accepts %j as %j', (input, expected) => {
    expect(normalizeGoalValue(input)).toBe(expected);
  });

  it.each([undefined, null, '', '   '])('treats %j as no value', (input) => {
    expect(normalizeGoalValue(input)).toBeUndefined();
  });

  it.each([
    'forty-nine',
    '49,90',
    '0x10',
    'Infinity',
    'NaN',
    true,
    {},
    [],
    Number.NaN,
    Number.POSITIVE_INFINITY,
    -1,
    MAX_GOAL_VALUE * 10,
  ])('throws on %j', (input) => {
    expect(() => normalizeGoalValue(input)).toThrow(/trackGoal value/);
  });
});

describe('normalizeGoalProperties', () => {
  it('converts numbers and booleans to strings and leaves strings alone', () => {
    expect(
      normalizeGoalProperties({ order_id: 12345, total: 49.9, first_order: true, gift: false, currency: 'EUR' }),
    ).toEqual({ order_id: '12345', total: '49.9', first_order: 'true', gift: 'false', currency: 'EUR' });
  });

  it('leaves out null and undefined entries', () => {
    expect(normalizeGoalProperties({ coupon: null, ref: undefined, plan: 'pro' })).toEqual({ plan: 'pro' });
  });

  it.each([undefined, null])('returns undefined for %j', (input) => {
    expect(normalizeGoalProperties(input)).toBeUndefined();
  });

  it.each([
    ['a nested object', { items: { sku: 'A1' } }],
    ['an array', { skus: ['A1', 'B2'] }],
    ['a non-finite number', { total: Number.NaN }],
    ['a function', { cb: () => 1 }],
  ])('throws on %s', (_label, input) => {
    expect(() => normalizeGoalProperties(input)).toThrow(/must be a string, number or boolean/);
  });

  it.each([['a string', 'order=1'], ['an array', ['a']], ['a number', 1]])(
    'throws when properties is %s',
    (_label, input) => {
      expect(() => normalizeGoalProperties(input)).toThrow(/must be an object/);
    },
  );
});

describe('trackGoal on the wire', () => {
  const fetchMock = (): Mock => globalThis.fetch as unknown as Mock;

  interface WireGoal {
    type: string;
    name?: string;
    value?: unknown;
    properties?: Record<string, unknown>;
  }

  const lastGoals = (): WireGoal[] => {
    const calls = fetchMock().mock.calls;
    expect(calls.length).toBeGreaterThan(0);
    const init = calls[calls.length - 1][1] as { body: string };
    const payload = JSON.parse(init.body) as { actions: WireGoal[] };
    return payload.actions.filter((a) => a.type === 'goal');
  };

  let sdk: NotifuseAnalyticsSDK;
  beforeEach(async () => {
    vi.useFakeTimers();
    sdk = new NotifuseAnalyticsSDK();
    await sdk.init({ workspace_id: 'ws_goals', endpoint: 'https://collector.example.com' });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('sends a numeric property and a string value with the server types', async () => {
    await sdk.trackGoal({
      action: 'purchase',
      type: 'purchase',
      value: '49.90',
      properties: { order_id: 12345, first_order: true },
    });

    const [goal] = lastGoals();
    expect(goal.value).toBe(49.9);
    expect(goal.properties).toEqual({ order_id: '12345', first_order: 'true' });
  });

  it('rejects an uncoercible goal without recording it, and later goals still go out', async () => {
    // Plain-JS callers bypass GoalData's types; cast to say so.
    const bad = { action: 'purchase', type: 'purchase', properties: { items: [{ sku: 'A1' }] } };
    await expect(sdk.trackGoal(bad as unknown as GoalData)).rejects.toThrow(/items/);

    await sdk.trackGoal({ action: 'newsletter', type: 'lead' });
    expect(lastGoals().map((g) => g.name)).toEqual(['newsletter']);
  });
});
