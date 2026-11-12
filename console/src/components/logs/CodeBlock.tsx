import { App } from 'antd'
import { Copy } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { copyToClipboard } from '../../lib/copyToClipboard'
import { formatCode } from './formatCode'

export interface CodeBlockProps {
  /** Text, or a value shown as indented JSON. */
  value: unknown
  /** Names the block for the copy button ("Copy payload"). */
  label: string
}

/** A scrolling code block of bounded height, with a copy button. */
export function CodeBlock({ value, label }: CodeBlockProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const text = formatCode(value)

  const copy = async () => {
    if (await copyToClipboard(text)) message.success(t`Copied`)
    else message.error(t`Failed to copy to clipboard`)
  }

  return (
    <div className="relative">
      <pre className="m-0 max-h-[400px] overflow-auto rounded-md border border-line-soft bg-subtle p-3 font-mono text-xs whitespace-pre-wrap break-all text-ink-2">
        {text}
      </pre>
      <button
        type="button"
        aria-label={t`Copy ${label}`}
        title={t`Copy`}
        onClick={() => void copy()}
        className="absolute top-1.5 right-1.5 flex items-center rounded-sm bg-surface p-1 text-ink-3 hover:bg-fill hover:text-ink"
      >
        <Copy size={12} aria-hidden="true" />
      </button>
    </div>
  )
}
