import { useEffect, useState } from 'react'
import { App, Button } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCopy } from '@fortawesome/free-regular-svg-icons'
import { faCheck } from '@fortawesome/free-solid-svg-icons'
import { Highlight, themes } from 'prism-react-renderer'
import { useLingui } from '@lingui/react/macro'

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

async function copyText(text: string): Promise<boolean> {
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

export interface CodeSnippetProps {
  code: string
  /** Prism grammar name, e.g. `markup` or `javascript`. */
  language: string
}

/** Syntax-highlighted code block with a copy button floating over its top-right corner. */
export function CodeSnippet({ code, language }: CodeSnippetProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [copied, setCopied] = useState(false)

  // Drop back to the copy icon so a second copy still gives feedback.
  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 2000)
    return () => window.clearTimeout(timer)
  }, [copied])

  const handleCopy = async () => {
    if (await copyText(code)) {
      setCopied(true)
      return
    }
    message.error(t`Failed to copy to clipboard`)
  }

  return (
    <div className="relative overflow-hidden rounded border border-gray-200">
      <Button
        size="small"
        onClick={handleCopy}
        className="!absolute right-2 top-2 z-10"
        icon={<FontAwesomeIcon icon={copied ? faCheck : faCopy} className="opacity-70" size="sm" />}
      >
        {copied ? t`Copied` : t`Copy`}
      </Button>
      <Highlight theme={themes.github} code={code} language={language}>
        {({ className, style, tokens, getLineProps, getTokenProps }) => (
          // The right padding keeps long lines from scrolling under the button.
          <pre className={`${className} !m-0 overflow-x-auto p-3 pr-24 text-xs`} style={style}>
            {tokens.map((line, index) => (
              <div key={index} {...getLineProps({ line })}>
                {line.map((token, key) => (
                  <span key={key} {...getTokenProps({ token })} />
                ))}
              </div>
            ))}
          </pre>
        )}
      </Highlight>
    </div>
  )
}
