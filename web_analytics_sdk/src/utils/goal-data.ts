/**
 * Goal value and properties as the server stores them: a number, and a flat
 * string map.
 *
 * trackGoal is mostly called from plain JS (snippets, tag managers), where
 * nothing enforces GoalData's types, so `properties: { order_id: 12345 }` and
 * `value: '49.90'` are ordinary calls. They are coerced here so the beat never
 * carries a wrong type. What has no faithful coercion throws, as setDimension
 * does for a non-string value, rather than reaching the server.
 */

/** Mirrors WebTrackMaxGoalValue in internal/domain/web_analytics.go. */
export const MAX_GOAL_VALUE = 1e12;

// JSON number syntax only: Number() would also accept '0x10', 'Infinity' and
// whitespace-only strings.
const NUMERIC_STRING = /^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$/;

/**
 * Returns the goal value as a number, or undefined when there is none.
 * undefined, null and '' mean no value; a numeric string is converted.
 */
export function normalizeGoalValue(value: unknown): number | undefined {
  if (value === undefined || value === null) return undefined;

  let num: number;
  if (typeof value === 'number') {
    num = value;
  } else if (typeof value === 'string') {
    const trimmed = value.trim();
    if (trimmed === '') return undefined;
    if (!NUMERIC_STRING.test(trimmed)) {
      throw new Error(`trackGoal value must be a number, got "${value}"`);
    }
    num = Number(trimmed);
  } else {
    throw new Error(`trackGoal value must be a number, got ${typeof value}`);
  }

  if (!Number.isFinite(num) || num < 0 || num > MAX_GOAL_VALUE) {
    throw new Error(`trackGoal value must be between 0 and ${MAX_GOAL_VALUE}`);
  }
  return num;
}

/**
 * Returns the properties as a flat string map, or undefined when there are
 * none. Numbers and booleans become strings, and a value with toJSON (a Date)
 * its JSON form; null and undefined entries are left out. Objects, arrays and
 * anything else throw.
 */
export function normalizeGoalProperties(
  properties: unknown,
): Record<string, string> | undefined {
  if (properties === undefined || properties === null) return undefined;
  if (typeof properties !== 'object' || Array.isArray(properties)) {
    throw new Error('trackGoal properties must be an object');
  }

  const out: Record<string, string> = {};
  for (const [key, entry] of Object.entries(properties as Record<string, unknown>)) {
    // What JSON.stringify sends: a Date, a URL or a decimal library's number
    // goes out as its toJSON() form, which reached the server as a string
    // before properties were checked here.
    const raw = hasToJSON(entry) ? entry.toJSON(key) : entry;
    if (raw === undefined || raw === null) continue;
    if (typeof raw === 'string') {
      out[key] = raw;
    } else if (typeof raw === 'boolean') {
      out[key] = String(raw);
    } else if (typeof raw === 'number' && Number.isFinite(raw)) {
      out[key] = String(raw);
    } else {
      throw new Error(
        `trackGoal property "${key}" must be a string, number or boolean`,
      );
    }
  }
  return out;
}

function hasToJSON(value: unknown): value is { toJSON(key: string): unknown } {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { toJSON?: unknown }).toJSON === 'function'
  );
}
