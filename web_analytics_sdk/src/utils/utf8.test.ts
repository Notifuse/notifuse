import { describe, it, expect, afterEach, vi } from 'vitest';
import { utf8Length } from './utf8';

// The server counts goal names and properties in bytes, after decoding JSON.
describe('utf8Length', () => {
  const cases: [string, string, number][] = [
    ['ASCII', 'abc', 3],
    ['a two-byte character', 'é', 2],
    ['a three-byte character', '€', 3],
    ['an emoji', '😀', 4],
    ['the last one-byte character', '\u007f', 1],
    ['the first two-byte character', '\u0080', 2],
    ['the last two-byte character', '\u07ff', 2],
    ['the first three-byte character', '\u0800', 3],
    ['the last three-byte character', '\uffff', 3],
    ['the first four-byte character', '\u{10000}', 4],
    // JSON decoding turns a lone surrogate into U+FFFD, three bytes.
    ['a lone surrogate', '\ud800', 3],
  ];

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each(cases)('counts %s', (_label, input, bytes) => {
    expect(utf8Length(input)).toBe(bytes);
  });

  it.each(cases)('counts %s without TextEncoder', (_label, input, bytes) => {
    vi.stubGlobal('TextEncoder', undefined);
    expect(utf8Length(input)).toBe(bytes);
  });

  // A page can replace TextEncoder with a stand-in that has no encode.
  it.each(cases)('counts %s with a broken TextEncoder', (_label, input, bytes) => {
    vi.stubGlobal('TextEncoder', class {});
    expect(utf8Length(input)).toBe(bytes);
  });
});
