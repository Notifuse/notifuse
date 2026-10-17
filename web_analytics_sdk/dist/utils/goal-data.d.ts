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
/** Mirrors WebTrackMaxGoalValue in internal/domain/web_analytics.go. */
export declare const MAX_GOAL_VALUE = 1000000000000;
/** Mirrors WebTrackMaxGoalNameLength, which counts UTF-8 bytes. */
export declare const MAX_GOAL_ACTION_BYTES = 100;
/** Mirror WebTrackMaxGoalPropertyKeys, WebTrackMaxGoalPropertyValueLength and
 * WebTrackMaxGoalPropertiesBytes (keys and values together), in UTF-8 bytes.
 * Written as plain numbers: a Go test reads them from this file. */
export declare const MAX_GOAL_PROPERTY_KEYS = 50;
export declare const MAX_GOAL_PROPERTY_VALUE_BYTES = 1024;
export declare const MAX_GOAL_PROPERTIES_BYTES = 8192;
/**
 * Returns the goal name the server keeps, or throws: a string, in the form
 * JSON.stringify sends it, that is not blank and fits its byte limit. The
 * server drops any other goal on its own.
 */
export declare function checkGoalAction(action: unknown): string;
/**
 * Returns the goal value as a number, or undefined when there is none.
 * undefined, null, '' and a non-finite number mean no value; a numeric string
 * is converted.
 */
export declare function normalizeGoalValue(value: unknown): number | undefined;
/**
 * Returns the properties as a flat string map, or undefined when there are
 * none. Each entry is stored as it was when JSON.stringify sent it: numbers
 * and booleans become strings, a value with toJSON (a Date) its JSON form,
 * null and non-finite numbers '' (the server stores JSON null as ''), and
 * undefined, functions and symbols are left out. Objects and arrays throw, and
 * so do properties over the server's limits, which drops such a goal.
 */
export declare function normalizeGoalProperties(properties: unknown): Record<string, string> | undefined;
