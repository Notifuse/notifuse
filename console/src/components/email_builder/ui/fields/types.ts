// The contract every field of the settings kit follows.

export interface FieldWriteOptions {
  // Consecutive writes with the same key make one undo step (see useCoalesceKey). Only typed
  // number fields pass one; a click (segmented, switch, colour, select) is one step.
  coalesceKey?: string
}

// A field writes its value (undefined = back to the inherited value) and, when typed, a key.
export type FieldWrite<T> = (value: T, options?: FieldWriteOptions) => void
