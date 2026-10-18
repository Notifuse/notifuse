/**
 * Length of a string in UTF-8 bytes.
 *
 * TextEncoder exists in every browser the SDK targets, but the SDK is dropped
 * into pages it does not control and globals do get stripped there, so a
 * missing or broken one degrades to hand-counting rather than throwing on a
 * call that used to work.
 */
export function utf8Length(value: string): number {
  try {
    return new TextEncoder().encode(value).length;
  } catch {
    // TextEncoder removed, or replaced with a stand-in that doesn't encode.
  }
  // for..of walks code points, so a surrogate pair arrives once as its combined
  // code point and is counted as the single 4-byte character it encodes to.
  let bytes = 0;
  for (const char of value) {
    const code = char.codePointAt(0)!;
    if (code <= 0x7f) bytes += 1;
    else if (code <= 0x7ff) bytes += 2;
    else if (code <= 0xffff) bytes += 3;
    else bytes += 4;
  }
  return bytes;
}
