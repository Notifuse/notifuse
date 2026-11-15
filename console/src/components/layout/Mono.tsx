import { useRef, type ReactNode } from 'react'
import { App } from 'antd'
import { Copy } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { copyToClipboard } from '../../lib/copyToClipboard'

export interface MonoProps {
  children: ReactNode
  /** Adds a copy button. */
  copy?: boolean
}

/** An identifier, URL or UTM shown inline in the code typeface. */
export function Mono({ children, copy }: MonoProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const codeRef = useRef<HTMLElement>(null)
  const text = typeof children === 'string' || typeof children === 'number' ? String(children) : undefined

  const handleCopy = async () => {
    // What is shown, whatever the children are made of
    const value = codeRef.current?.textContent ?? ''
    if (await copyToClipboard(value)) message.success(t`Copied`)
    else message.error(t`Failed to copy to clipboard`)
  }

  return (
    <span className="inline-flex max-w-full min-w-0 items-center gap-1">
      <code ref={codeRef} title={text} className="truncate rounded-sm bg-fill px-1.5 font-mono text-xs text-ink-2">
        {children}
      </code>
      {copy ? (
        <button
          type="button"
          aria-label={t`Copy`}
          title={t`Copy`}
          onClick={handleCopy}
          className="flex shrink-0 items-center rounded-sm p-0.5 text-ink-3 hover:bg-fill hover:text-ink focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none"
        >
          <Copy size={12} aria-hidden="true" />
        </button>
      ) : null}
    </span>
  )
}
