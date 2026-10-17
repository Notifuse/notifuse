import { describe, it, expect, beforeEach, afterEach, vi, type Mock } from 'vitest';
import vm from 'node:vm';
import { NotifuseAnalyticsSDK } from '../sdk';
import type { GoalData } from '../types';
import {
  MAX_GOAL_ACTION_BYTES,
  MAX_GOAL_PROPERTIES_BYTES,
  MAX_GOAL_PROPERTY_KEYS,
  MAX_GOAL_PROPERTY_VALUE_BYTES,
  MAX_GOAL_VALUE,
  checkGoalAction,
  normalizeGoalProperties,
  normalizeGoalValue,
} from './goal-data';

/** A value made in another realm, as an iframe's would be. */
const fromAnotherFrame = (source: string): unknown => vm.runInNewContext(source);

/** BigInt.prototype.toJSON, the polyfill pages add so JSON.stringify accepts a BigInt. */
function withBigIntToJSON(run: () => void): void {
  const proto = BigInt.prototype as unknown as { toJSON?: () => string };
  proto.toJSON = function (this: bigint) {
    return this.toString();
  };
  try {
    run();
  } finally {
    delete proto.toJSON;
  }
}

// The limits mirror constants in internal/domain/web_analytics.go. Pinned as
// literals so a change here is deliberate; the Go side is compared with this
// file by internal/domain/web_analytics_goal_limits_contract_test.go.
describe('goal limits', () => {
  it('are the values the server enforces', () => {
    expect(MAX_GOAL_VALUE).toBe(1e12);
    expect(MAX_GOAL_ACTION_BYTES).toBe(100);
    expect(MAX_GOAL_PROPERTY_KEYS).toBe(50);
    expect(MAX_GOAL_PROPERTY_VALUE_BYTES).toBe(1024);
    expect(MAX_GOAL_PROPERTIES_BYTES).toBe(8192);
  });
});

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
    // The server's bound is inclusive.
    [1e12, 1e12],
    ['1e12', 1e12],
  ])('accepts %j as %j', (input, expected) => {
    expect(normalizeGoalValue(input)).toBe(expected);
  });

  // JSON.stringify sent these as the number, so the server stored it.
  it.each([
    ['a Number object', new Number(5), 5],
    ['a String object', new String('49.90'), 49.9],
    ['a value with toJSON', { toJSON: () => 5 }, 5],
    ['a function with toJSON', Object.assign(() => 0, { toJSON: () => 5 }), 5],
    ['a Number object from another frame', fromAnotherFrame('new Number(5)'), 5],
    ['a Number object with its own Symbol.toPrimitive', Object.assign(new Number(5), { [Symbol.toPrimitive]: () => 7 }), 7],
    ['a String object with its own toString', Object.assign(new String('5'), { toString: () => '7' }), 7],
    // toJSON comes first, as in JSON.stringify, then the unwrapping.
    ['a Number object with toJSON', Object.assign(new Number(5), { toJSON: () => 7 }), 7],
    // Blank for Go's strings.TrimSpace, which the server trims a string value with.
    ['a numeric string after U+0085', '\u008549.90', 49.9],
    ['a numeric string before U+0085', '49.90\u0085', 49.9],
  ])('accepts %s as its JSON form', (_label, input, expected) => {
    expect(normalizeGoalValue(input)).toBe(expected);
  });

  it('calls toJSON with the key "value", as JSON.stringify does', () => {
    expect(normalizeGoalValue({ toJSON: (key: string) => (key === 'value' ? 5 : -1) })).toBe(5);
  });

  it.each([undefined, null, '', '   ', '\u0085'])('treats %j as no value', (input) => {
    expect(normalizeGoalValue(input)).toBeUndefined();
  });

  // JSON.stringify left these out, so the goal was recorded without a value.
  it.each([
    ['a function', () => 5],
    ['a symbol', Symbol('value')],
  ])('treats %s as no value', (_label, input) => {
    expect(normalizeGoalValue(input)).toBeUndefined();
  });

  // JSON.stringify sends these as null, which the server reads as no value.
  it.each([Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY])(
    'treats the number %s as no value',
    (input) => {
      expect(normalizeGoalValue(input)).toBeUndefined();
    },
  );

  // Labelled rows: it.each spreads an unlabelled array row into the title.
  it.each([
    ['a word', 'forty-nine'],
    ['a decimal comma', '49,90'],
    ['a hex string', '0x10'],
    ['the string Infinity', 'Infinity'],
    ['the string NaN', 'NaN'],
    ['a string that overflows', '1e400'],
    ['a boolean', true],
    ['an object', {}],
    ['an array', []],
    ['a Date', new Date(Date.UTC(2026, 0, 2))],
    ['an object made from Number.prototype', Object.create(Number.prototype)],
    ['a negative number', -1],
    ['a number above the maximum', MAX_GOAL_VALUE * 10],
    ['a number just above the maximum', MAX_GOAL_VALUE + 1],
  ])('throws on %s', (_label, input) => {
    expect(() => normalizeGoalValue(input)).toThrow(/trackGoal value/);
  });

  it('names the value it rejects as out of range', () => {
    expect(() => normalizeGoalValue(-1)).toThrow(/between 0 and \d+, got -1$/);
  });

  // Kept out of the describe that fakes timers, which fakes performance too.
  // A value can come from a form field, and it is trimmed on the page's main
  // thread. A trailing-blank regexp rescans the blanks from every position,
  // which takes seconds on a run this long; walking the string takes
  // milliseconds.
  it('trims a long run of blanks in linear time', () => {
    const blanks = ' '.repeat(100_000);
    const started = performance.now();
    expect(() => normalizeGoalValue(`1${blanks}2`)).toThrow(/must be a number/);
    expect(normalizeGoalValue(`${blanks}5${blanks}`)).toBe(5);
    expect(performance.now() - started).toBeLessThan(500);
  });
});

describe('normalizeGoalProperties', () => {
  it('converts numbers and booleans to strings and leaves strings alone', () => {
    expect(
      normalizeGoalProperties({ order_id: 12345, total: 49.9, first_order: true, gift: false, currency: 'EUR' }),
    ).toEqual({ order_id: '12345', total: '49.9', first_order: 'true', gift: 'false', currency: 'EUR' });
  });

  // A Date, a URL or a decimal library's number reached the server through
  // its toJSON before properties were checked, and was stored as that string.
  it('sends a value with toJSON, such as a Date, as its JSON form', () => {
    const at = new Date(Date.UTC(2026, 0, 2));
    const total = { toJSON: () => 49.9 };
    expect(normalizeGoalProperties({ at, total, gone: { toJSON: () => undefined } })).toEqual({
      at: '2026-01-02T00:00:00.000Z',
      total: '49.9',
    });
    expect(() => normalizeGoalProperties({ nested: { toJSON: () => ({ sku: 'A1' }) } })).toThrow(
      /must be a string, number or boolean/,
    );
  });

  // JSON.stringify sent null, NaN and Infinity as null, which the server
  // stores as '', and left out undefined, functions and symbols.
  it('stores null and non-finite entries as the server did, and leaves out what JSON left out', () => {
    expect(
      normalizeGoalProperties({
        coupon: null,
        ref: undefined,
        total: Number.NaN,
        cap: Number.POSITIVE_INFINITY,
        at: new Date(Number.NaN),
        cb: () => 1,
        tag: Symbol('tag'),
        plan: 'pro',
      }),
    ).toEqual({ coupon: '', total: '', cap: '', at: '', plan: 'pro' });
  });

  it('unwraps Number, String and Boolean objects, as JSON.stringify does', () => {
    expect(
      normalizeGoalProperties({ n: new Number(5), s: new String('pro'), b: new Boolean(false) }),
    ).toEqual({ n: '5', s: 'pro', b: 'false' });
  });

  it('unwraps Number, String and Boolean objects made in another frame', () => {
    expect(
      normalizeGoalProperties({
        n: fromAnotherFrame('new Number(5)'),
        s: fromAnotherFrame('new String("pro")'),
        b: fromAnotherFrame('new Boolean(false)'),
      }),
    ).toEqual({ n: '5', s: 'pro', b: 'false' });
  });

  // JSON.stringify converts a String object with toString, and reads a Boolean
  // object's value directly, whatever its valueOf says.
  it('converts boxed values the way JSON.stringify does', () => {
    expect(
      normalizeGoalProperties({
        s: Object.assign(new String('x'), { toString: () => 'y' }),
        b: Object.assign(new Boolean(false), { valueOf: () => true }),
      }),
    ).toEqual({ s: 'y', b: 'false' });
  });

  it('calls toJSON with the key it is stored under, as JSON.stringify does', () => {
    const keyOf = { toJSON: (key: string) => key };
    expect(normalizeGoalProperties({ at: keyOf })).toEqual({ at: 'at' });
    expect(normalizeGoalProperties({ toJSON: (key: string) => ({ key }) })).toEqual({
      key: 'properties',
    });
  });

  it('sends a BigInt as its toJSON form when the page adds one', () => {
    withBigIntToJSON(() => {
      expect(normalizeGoalProperties({ order_id: BigInt(12345) })).toEqual({ order_id: '12345' });
    });
  });

  it('keeps an own "__proto__" key as an entry', () => {
    const parsed: unknown = JSON.parse('{"__proto__":"x","plan":"pro"}');
    const out = normalizeGoalProperties(parsed);
    expect(Object.keys(out ?? {})).toEqual(['__proto__', 'plan']);
    expect(JSON.stringify(out)).toBe('{"__proto__":"x","plan":"pro"}');
  });

  // The SDK sent properties only when they were truthy.
  it.each([
    ['undefined', undefined],
    ['null', null],
    ['false', false],
    ['an empty string', ''],
    ['zero', 0],
    ['NaN', Number.NaN],
  ])('returns undefined when properties is %s', (_label, input) => {
    expect(normalizeGoalProperties(input)).toBeUndefined();
  });

  // JSON.stringify left these out, or sent null, so no properties were sent.
  it.each([
    ['a function', () => ({ plan: 'pro' })],
    ['a symbol', Symbol('properties')],
    ['an object whose toJSON returns undefined', { plan: 'pro', toJSON: () => undefined }],
    ['an object whose toJSON returns null', { plan: 'pro', toJSON: () => null }],
  ])('returns undefined when properties is %s', (_label, input) => {
    expect(normalizeGoalProperties(input)).toBeUndefined();
  });

  // The server drops a goal whose properties are over any of its limits.
  describe('limits', () => {
    const entries = (count: number, value = 'v'): Record<string, string> =>
      Object.fromEntries(Array.from({ length: count }, (_, i) => [`k${i}`, value]));
    // Keys of 3 bytes and values of 1021 make entries of 1024 bytes each.
    const sized = (extraBytes: number): Record<string, string> => {
      const out: Record<string, string> = {};
      const count = MAX_GOAL_PROPERTIES_BYTES / 1024;
      for (let i = 0; i < count; i++) out[`é${i}`] = 'v'.repeat(1021);
      out['é0'] += 'v'.repeat(extraBytes);
      return out;
    };

    it('accepts properties at each limit', () => {
      expect(Object.keys(normalizeGoalProperties(entries(MAX_GOAL_PROPERTY_KEYS)) ?? {})).toHaveLength(
        MAX_GOAL_PROPERTY_KEYS,
      );
      expect(
        normalizeGoalProperties({ note: 'é'.repeat(MAX_GOAL_PROPERTY_VALUE_BYTES / 2) }),
      ).toBeDefined();
      expect(normalizeGoalProperties(sized(0))).toBeDefined();
    });

    it.each([
      ['too many keys', entries(MAX_GOAL_PROPERTY_KEYS + 1), /exceed \d+ keys/],
      ['a value over the byte limit', { note: 'é'.repeat(MAX_GOAL_PROPERTY_VALUE_BYTES / 2) + 'a' }, /"note" exceeds \d+ bytes/],
      ['keys and values over the total byte limit', sized(1), /properties exceed \d+ bytes/],
    ])('throws on %s', (_label, input, reason) => {
      expect(() => normalizeGoalProperties(input)).toThrow(reason);
    });

    // Left out entries are not sent, so they do not count.
    it('counts only the entries it sends', () => {
      const props: Record<string, unknown> = entries(MAX_GOAL_PROPERTY_KEYS);
      props.gone = undefined;
      expect(Object.keys(normalizeGoalProperties(props) ?? {})).toHaveLength(MAX_GOAL_PROPERTY_KEYS);
    });
  });

  it.each([
    ['a nested object', { items: { sku: 'A1' } }],
    ['an array', { skus: ['A1', 'B2'] }],
    ['a bigint', { id: BigInt(1) }],
  ])('throws on %s', (_label, input) => {
    expect(() => normalizeGoalProperties(input)).toThrow(/must be a string, number or boolean/);
  });

  it.each([
    ['a string', 'order=1'],
    ['an array', ['a']],
    ['a number', 1],
    ['a String object', new String('order=1')],
  ])('throws when properties is %s', (_label, input) => {
    expect(() => normalizeGoalProperties(input)).toThrow(/must be an object/);
  });
});

describe('checkGoalAction', () => {
  it.each([
    ['a name', 'purchase'],
    ['a name at the byte limit', 'a'.repeat(MAX_GOAL_ACTION_BYTES)],
    ['a multi-byte name at the byte limit', 'é'.repeat(MAX_GOAL_ACTION_BYTES / 2)],
    // A lone surrogate reaches the server as U+FFFD, three bytes.
    ['lone surrogates at the byte limit', '\ud800'.repeat(33) + 'a'],
    // Go's strings.TrimSpace keeps these, so the server stores such a name.
    ['a name of U+FEFF', '\ufeff'],
    ['a name of U+180E', '\u180e'],
    ['a name of U+200B', '\u200b'],
  ])('returns %s', (_label, input) => {
    expect(checkGoalAction(input)).toBe(input);
  });

  // JSON.stringify sent these as the string, so the server stored it.
  it.each([
    ['a String object', new String('purchase')],
    ['a value with toJSON', { toJSON: () => 'purchase' }],
    ['a value whose toJSON reads its key, "name"', { toJSON: (key: string) => (key === 'name' ? 'purchase' : '') }],
  ])('returns %s as its JSON form', (_label, input) => {
    expect(checkGoalAction(input)).toBe('purchase');
  });

  // Every character Go's unicode.IsSpace reports, which strings.TrimSpace
  // removes, so the server drops a name made of them.
  const goSpaces = [
    0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x20, 0x85, 0xa0, 0x1680,
    0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007, 0x2008, 0x2009, 0x200a,
    0x2028, 0x2029, 0x202f, 0x205f, 0x3000,
  ];
  it.each(goSpaces.map((code) => [`U+${code.toString(16).toUpperCase().padStart(4, '0')}`, code]))(
    'throws on a name of %s',
    (_label, code) => {
      expect(() => checkGoalAction(String.fromCharCode(code as number).repeat(2))).toThrow(
        /requires an action/,
      );
    },
  );

  it.each([
    ['undefined', undefined],
    ['an empty string', ''],
    ['a blank string', '   '],
    ['a number', 5],
  ])('throws on %s', (_label, input) => {
    expect(() => checkGoalAction(input)).toThrow(/requires an action/);
  });

  // The server counts bytes, so a name short in characters can still be over.
  it.each([
    ['a name over the byte limit', 'a'.repeat(MAX_GOAL_ACTION_BYTES + 1)],
    ['a multi-byte name over the byte limit', 'é'.repeat(MAX_GOAL_ACTION_BYTES / 2 + 1)],
    ['lone surrogates over the byte limit', '\ud800'.repeat(33) + 'ab'],
  ])('throws on %s', (_label, input) => {
    expect(() => checkGoalAction(input)).toThrow(/exceeds \d+ bytes/);
  });
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

  // parseFloat('€49.90') is NaN. JSON.stringify sent it as null, which the
  // server reads as no value, so the conversion was recorded; it must still be.
  it.each([Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY])(
    'records a goal whose value is %s, with no value',
    async (value) => {
      await sdk.trackGoal({ action: 'purchase', type: 'purchase', value });

      const [goal] = lastGoals();
      expect(goal.name).toBe('purchase');
      expect(goal).not.toHaveProperty('value');
    },
  );

  // JSON.stringify sent NaN as null, which the server stores as ''.
  it('records a goal with a null or non-finite property, sending it as the empty string the server stored', async () => {
    await sdk.trackGoal({
      action: 'purchase',
      type: 'purchase',
      properties: { total: Number.NaN, coupon: null, plan: 'pro' },
    });

    const [goal] = lastGoals();
    expect(goal.name).toBe('purchase');
    expect(goal.properties).toEqual({ total: '', coupon: '', plan: 'pro' });
  });

  // `properties: coupon && { coupon }` with no coupon is an ordinary call; the
  // goal was recorded without properties and must still be.
  it.each([
    ['false', false],
    ['an empty string', ''],
    ['zero', 0],
  ])('records a goal whose properties is %s, without properties', async (_label, properties) => {
    await sdk.trackGoal({ action: 'purchase', type: 'purchase', properties } as unknown as GoalData);

    const [goal] = lastGoals();
    expect(goal.name).toBe('purchase');
    expect(goal).not.toHaveProperty('properties');
  });

  it('records a goal with a function property, leaving it out as JSON.stringify did', async () => {
    await sdk.trackGoal({
      action: 'purchase',
      type: 'purchase',
      properties: { plan: 'pro', cb: () => 1 },
    } as unknown as GoalData);

    expect(lastGoals()[0].properties).toEqual({ plan: 'pro' });
  });

  // Read once: a getter that changes its answer must not pass the check with
  // one name and be recorded with another.
  it('records the action as it was checked', async () => {
    let reads = 0;
    const data = {
      type: 'purchase',
      get action() {
        reads++;
        return reads === 1 ? 'purchase' : '';
      },
    };
    await sdk.trackGoal(data as unknown as GoalData);

    expect(lastGoals().map((g) => g.name)).toEqual(['purchase']);
  });

  it('records the type as it was checked', async () => {
    let reads = 0;
    const data = {
      action: 'purchase',
      get type() {
        reads++;
        return reads === 1 ? 'purchase' : 'not-a-type';
      },
    };
    await sdk.trackGoal(data as unknown as GoalData);

    expect(lastGoals()[0]).toMatchObject({ name: 'purchase', goal_type: 'purchase' });
  });

  it('records a String object action as its string', async () => {
    await sdk.trackGoal({ action: new String('purchase'), type: 'purchase' } as unknown as GoalData);

    expect(lastGoals().map((g) => g.name)).toEqual(['purchase']);
  });

  // A page whose sessionStorage is full fails every persist. That warning is
  // for debugging: outside debug mode it would repeat in every visitor's
  // console on every beat.
  it.each([
    ['without debug', false],
    ['in debug mode', true],
  ])('warns about a failed persist only in debug mode: %s', async (_label, debug) => {
    const page = new NotifuseAnalyticsSDK();
    await page.init({ workspace_id: 'ws_goals', endpoint: 'https://collector.example.com', debug });
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const setItem = vi.spyOn(window.sessionStorage, 'setItem').mockImplementation(() => {
      throw new DOMException('quota', 'QuotaExceededError');
    });
    try {
      await page.trackGoal({ action: 'purchase', type: 'purchase' });
      const persistWarnings = warn.mock.calls.filter((call) => String(call[0]).includes('Failed to persist'));
      expect(persistWarnings.length > 0).toBe(debug);
    } finally {
      setItem.mockRestore();
      warn.mockRestore();
    }
  });

  it('sends a Date property as the string JSON.stringify always sent', async () => {
    const at = new Date(Date.UTC(2026, 0, 2));
    // GoalData does not admit a Date; plain-JS callers pass one anyway.
    await sdk.trackGoal({ action: 'booking', type: 'booking', properties: { at } } as unknown as GoalData);

    const [goal] = lastGoals();
    expect(goal.properties).toEqual({ at: '2026-01-02T00:00:00.000Z' });
  });

  // The call resolved for these before they were checked here, and a page that
  // awaits trackGoal before redirecting must still redirect.
  it.each([
    ['a nested property', { action: 'purchase', type: 'purchase', properties: { items: [{ sku: 'A1' }] } }, /items/],
    ['a value that is not a number', { action: 'purchase', type: 'purchase', value: 'abc' }, /value/],
    ['a negative value', { action: 'purchase', type: 'purchase', value: -5 }, /between 0/],
    ['a value above the maximum', { action: 'purchase', type: 'purchase', value: MAX_GOAL_VALUE * 10 }, /between 0/],
    ['an empty action', { action: '', type: 'purchase' }, /requires an action/],
    ['an action over the byte limit', { action: 'a'.repeat(MAX_GOAL_ACTION_BYTES + 1), type: 'purchase' }, /bytes/],
    [
      'properties over the key limit',
      {
        action: 'purchase',
        type: 'purchase',
        properties: Object.fromEntries(Array.from({ length: MAX_GOAL_PROPERTY_KEYS + 1 }, (_, i) => [`k${i}`, 'v'])),
      },
      /keys/,
    ],
    [
      'properties whose getter throws something that is not an Error',
      {
        action: 'purchase',
        type: 'purchase',
        get properties(): never {
          throw null;
        },
      },
      /^\[NotifuseAnalytics\] reading the goal data threw/,
    ],
  ])('resolves on %s without recording it, warns, and later goals still go out', async (_label, bad, reason) => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    try {
      // Plain-JS callers bypass GoalData's types; cast to say so.
      await expect(sdk.trackGoal(bad as unknown as GoalData)).resolves.toBeUndefined();
      expect(warn).toHaveBeenCalledWith(expect.stringMatching(reason));
      expect(warn).toHaveBeenCalledWith(expect.stringMatching(/the goal was not recorded$/));
    } finally {
      warn.mockRestore();
    }

    await sdk.trackGoal({ action: 'newsletter', type: 'lead' });
    expect(lastGoals().map((g) => g.name)).toEqual(['newsletter']);
  });
});
