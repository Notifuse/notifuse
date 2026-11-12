/**
 * Copies through a throwaway textarea.
 *
 * `document.execCommand` is deprecated, but it is the only copy a page gets on
 * an insecure origin — and a self-hosted console is routinely reached over
 * plain http on a LAN address, which is exactly when the install snippet
 * matters most.
 */
function copyViaSelection(text: string): boolean {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  // Off-screen rather than hidden: display:none cannot be selected.
  textarea.style.position = 'fixed'
  textarea.style.top = '-9999px'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)

  const selection = document.getSelection()
  const previous = selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : null

  try {
    textarea.select()
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    document.body.removeChild(textarea)
    // Copying must not steal whatever the reader had highlighted.
    if (selection && previous) {
      selection.removeAllRanges()
      selection.addRange(previous)
    }
  }
}

/**
 * Copies text to the clipboard: through the clipboard API when the page may use it, otherwise
 * through a selection, which also works on a console served over plain http. Resolves to whether
 * the text was copied.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // Insecure origin, unfocused document, denied permission — all recoverable
    // through the selection path below.
  }
  return copyViaSelection(text)
}
