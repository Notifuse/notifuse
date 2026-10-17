/**
 * Length of a string in UTF-8 bytes.
 *
 * TextEncoder exists in every browser the SDK targets, but the SDK is dropped
 * into pages it does not control and globals do get stripped there, so a
 * missing or broken one degrades to hand-counting rather than throwing on a
 * call that used to work.
 */
export declare function utf8Length(value: string): number;
