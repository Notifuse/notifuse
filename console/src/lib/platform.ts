/** A Mac (its keyboard shortcuts read ⌘ rather than Ctrl). */
export function isMac(): boolean {
  return typeof navigator !== 'undefined' && /mac/i.test(navigator.platform)
}
