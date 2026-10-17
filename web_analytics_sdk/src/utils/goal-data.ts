/**
 * Goal action, value and properties as the server stores them: a name, a
 * number, and a flat string map.
 *
 * trackGoal is mostly called from plain JS (snippets, tag managers), where
 * nothing enforces GoalData's types, so `properties: { order_id: 12345 }` and
 * `value: '49.90'` are ordinary calls. They are coerced here so the beat never
 * carries a wrong type. Whatever JSON.stringify used to send and the server
 * stored still is, in the same form; what has no faithful coercion throws,
 * and trackGoal drops that goal with a warning.
 */

import { utf8Length } from './utf8';

/** Mirrors WebTrackMaxGoalValue in internal/domain/web_analytics.go. */
export const MAX_GOAL_VALUE = 1e12;

/** Mirrors WebTrackMaxGoalNameLength, which counts UTF-8 bytes. */
export const MAX_GOAL_ACTION_BYTES = 100;

/** Mirror WebTrackMaxGoalPropertyKeys, WebTrackMaxGoalPropertyValueLength and
 * WebTrackMaxGoalPropertiesBytes (keys and values together), in UTF-8 bytes.
 * Written as plain numbers: a Go test reads them from this file. */
export const MAX_GOAL_PROPERTY_KEYS = 50;
export const MAX_GOAL_PROPERTY_VALUE_BYTES = 1024;
export const MAX_GOAL_PROPERTIES_BYTES = 8192;

// JSON number syntax only: Number() would also accept '0x10', 'Infinity' and
// whitespace-only strings.
const NUMERIC_STRING = /^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$/;

// What Go's strings.TrimSpace removes. String.prototype.trim differs on two
// characters: it removes U+FEFF, which Go keeps, and keeps U+0085, which Go
// removes. A Go test compares this class with unicode.IsSpace.
const GO_BLANK = /^[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]*$/;

/**
 * Returns the goal name the server keeps, or throws: a string, in the form
 * JSON.stringify sends it, that is not blank and fits its byte limit. The
 * server drops any other goal on its own.
 */
export function checkGoalAction(action: unknown): string {
  const name = jsonForm(action, 'name');
  if (typeof name !== 'string' || GO_BLANK.test(name)) {
    throw new Error('trackGoal requires an action, a non-empty string');
  }
  if (utf8Length(name) > MAX_GOAL_ACTION_BYTES) {
    throw new Error(`trackGoal action exceeds ${MAX_GOAL_ACTION_BYTES} bytes`);
  }
  return name;
}

/**
 * Returns the goal value as a number, or undefined when there is none.
 * undefined, null, '' and a non-finite number mean no value; a numeric string
 * is converted.
 */
export function normalizeGoalValue(value: unknown): number | undefined {
  const v = jsonForm(value, 'value');
  // JSON.stringify leaves out a function or a symbol, so the goal was
  // recorded without a value.
  if (v === undefined || v === null || typeof v === 'function' || typeof v === 'symbol') {
    return undefined;
  }

  let num: number;
  if (typeof v === 'number') {
    // NaN is what parseFloat('€49.90') gives. JSON.stringify sent it as null,
    // which the server reads as no value, and the goal was still recorded.
    if (!Number.isFinite(v)) return undefined;
    num = v;
  } else if (typeof v === 'string') {
    // Trimmed of what either JS or Go counts as blank: the server trims with
    // strings.TrimSpace, and the SDK sends the number, so a value is never
    // skipped here that the server would keep.
    const trimmed = trimBlank(v);
    if (trimmed === '') return undefined;
    if (!NUMERIC_STRING.test(trimmed)) {
      throw new Error(`trackGoal value must be a number, got "${v}"`);
    }
    num = Number(trimmed);
  } else {
    throw new Error(`trackGoal value must be a number, got ${typeof v}`);
  }

  // A numeric string can still overflow to Infinity ('1e400'), which is out of
  // range like any other value over the limit.
  if (num < 0 || num > MAX_GOAL_VALUE) {
    throw new Error(`trackGoal value must be between 0 and ${MAX_GOAL_VALUE}, got ${num}`);
  }
  return num;
}

// Every character either JS's \s or Go's unicode.IsSpace counts as blank is a
// single UTF-16 code unit, so the string is walked one unit at a time. A
// trailing-blank regexp would rescan the blanks from every position, which a
// long value from a form field makes slow enough to freeze the page.
const BLANK_CHAR = /[\s\u0085]/;

function trimBlank(value: string): string {
  let start = 0;
  let end = value.length;
  while (start < end && BLANK_CHAR.test(value[start])) start++;
  while (end > start && BLANK_CHAR.test(value[end - 1])) end--;
  return value.slice(start, end);
}

/**
 * Returns the properties as a flat string map, or undefined when there are
 * none. Each entry is stored as it was when JSON.stringify sent it: numbers
 * and booleans become strings, a value with toJSON (a Date) its JSON form,
 * null and non-finite numbers '' (the server stores JSON null as ''), and
 * undefined, functions and symbols are left out. Objects and arrays throw, and
 * so do properties over the server's limits, which drops such a goal.
 */
export function normalizeGoalProperties(
  properties: unknown,
): Record<string, string> | undefined {
  // The SDK sent properties only when they were truthy, so false, '', 0 and
  // NaN meant none.
  if (!properties) return undefined;
  const obj = jsonForm(properties, 'properties');
  if (obj === undefined || obj === null || typeof obj === 'function' || typeof obj === 'symbol') {
    return undefined;
  }
  if (typeof obj !== 'object' || Array.isArray(obj)) {
    throw new Error('trackGoal properties must be an object');
  }

  // No prototype, so an own "__proto__" key is kept as an entry.
  const out = Object.create(null) as Record<string, string>;
  for (const [key, entry] of Object.entries(obj as Record<string, unknown>)) {
    const raw = jsonForm(entry, key);
    if (raw === undefined || typeof raw === 'function' || typeof raw === 'symbol') continue;
    if (raw === null || (typeof raw === 'number' && !Number.isFinite(raw))) {
      out[key] = '';
    } else if (typeof raw === 'string') {
      out[key] = raw;
    } else if (typeof raw === 'boolean' || typeof raw === 'number') {
      out[key] = String(raw);
    } else {
      throw new Error(
        `trackGoal property "${key}" must be a string, number or boolean`,
      );
    }
  }
  checkGoalPropertiesSize(out);
  return out;
}

// Keys are counted as JS holds them. Two keys that differ only in an unpaired
// surrogate are one key on the server, whose JSON decoder turns each into
// U+FFFD, and are counted twice here; no page sends such keys on purpose.
function checkGoalPropertiesSize(properties: Record<string, string>): void {
  const keys = Object.keys(properties);
  if (keys.length > MAX_GOAL_PROPERTY_KEYS) {
    throw new Error(`trackGoal properties exceed ${MAX_GOAL_PROPERTY_KEYS} keys`);
  }
  let total = 0;
  for (const key of keys) {
    const valueBytes = utf8Length(properties[key]);
    if (valueBytes > MAX_GOAL_PROPERTY_VALUE_BYTES) {
      throw new Error(
        `trackGoal property "${key}" exceeds ${MAX_GOAL_PROPERTY_VALUE_BYTES} bytes`,
      );
    }
    total += utf8Length(key) + valueBytes;
  }
  if (total > MAX_GOAL_PROPERTIES_BYTES) {
    throw new Error(`trackGoal properties exceed ${MAX_GOAL_PROPERTIES_BYTES} bytes`);
  }
}

/**
 * What JSON.stringify sends for value: its toJSON() form (a Date, a URL, a
 * decimal library's number, a BigInt under the usual polyfill), then a Number,
 * String or Boolean object unwrapped to its primitive.
 */
function jsonForm(value: unknown, key: string): unknown {
  let v = value;
  if ((typeof v === 'object' && v !== null) || typeof v === 'function' || typeof v === 'bigint') {
    const toJSON = (v as { toJSON?: unknown }).toJSON;
    if (typeof toJSON === 'function') v = toJSON.call(v, key);
  }
  if (typeof v !== 'object' || v === null) return v;
  // Recognised by the value it wraps, as JSON.stringify does, so a boxed value
  // made in another frame counts, and converted the same way: a Number or
  // String object through its valueOf or toString, a Boolean object directly.
  if (wraps(Number.prototype.valueOf, v)) return +(v as unknown as number);
  if (wraps(String.prototype.valueOf, v)) return String(v);
  if (wraps(Boolean.prototype.valueOf, v)) return Boolean.prototype.valueOf.call(v);
  return v;
}

/** Whether v holds the primitive that valueOf, a built-in one, reads. */
function wraps(valueOf: (this: unknown) => unknown, v: object): boolean {
  try {
    valueOf.call(v);
    return true;
  } catch {
    return false;
  }
}
