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
export declare const MAX_GOAL_VALUE = 1000000000000;
/**
 * Returns the goal value as a number, or undefined when there is none.
 * undefined, null, '' and a non-finite number mean no value; a numeric string
 * is converted.
 */
export declare function normalizeGoalValue(value: unknown): number | undefined;
/**
 * Returns the properties as a flat string map, or undefined when there are
 * none. Numbers and booleans become strings, and a value with toJSON (a Date)
 * its JSON form; null, undefined and non-finite entries are left out, as
 * JSON.stringify sends those as null. Objects, arrays and anything else throw.
 */
export declare function normalizeGoalProperties(properties: unknown): Record<string, string> | undefined;
