// The API limits a saved block's name to 255 bytes of UTF-8 (Go's len), not 255 characters
export const MAX_BLOCK_NAME_BYTES = 255

const encoder = new TextEncoder()

export const utf8Length = (text: string) => encoder.encode(text).length

// The longest start of text, in whole characters, that fits in max bytes
export const fitUtf8 = (text: string, max: number): string => {
  let out = ''
  let bytes = 0
  for (const char of text) {
    const size = utf8Length(char)
    if (bytes + size > max) break
    out += char
    bytes += size
  }
  return out
}

// The name of a duplicate: the original, cut if needed, then the suffix, within the API's limit
export const copyName = (name: string, suffix: string): string =>
  fitUtf8(name, MAX_BLOCK_NAME_BYTES - utf8Length(suffix)).trimEnd() + suffix
