import { Button, Checkbox, App } from 'antd'
import { Copy, Download, Printer } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { useBrandName } from '../../hooks/useBrandName'
import { getInstanceBranding } from '../brand/instanceBranding'

export interface RecoveryCodesListProps {
  codes: string[]
  /** The person's email, named in the file and the printout. */
  email?: string
  saved: boolean
  onSavedChange: (saved: boolean) => void
}

function codesText(codes: string[], heading: string): string {
  return `${heading}\n\n${codes.join('\n')}\n`
}

/**
 * The recovery codes, shown once: copy, download as .txt, print, and a box to tick before the
 * caller lets the person go on.
 */
export function RecoveryCodesList({ codes, email, saved, onSavedChange }: RecoveryCodesListProps) {
  const { t } = useLingui()
  const brand = useBrandName()
  const { message } = App.useApp()
  const account = email ?? ''
  const heading = account ? t`${brand} recovery codes for ${account}` : t`${brand} recovery codes`

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(codes.join('\n'))
      message.success(t`Recovery codes copied`)
    } catch {
      message.error(t`Could not copy the codes. Select them and copy by hand.`)
    }
  }

  const download = () => {
    const url = URL.createObjectURL(new Blob([codesText(codes, heading)], { type: 'text/plain' }))
    const link = document.createElement('a')
    link.href = url
    // No product name in the file name under the instance's own brand.
    link.download = getInstanceBranding().custom ? 'recovery-codes.txt' : 'notifuse-recovery-codes.txt'
    // Attached and revoked later: some browsers ignore a click on a detached link, or start
    // the download after a revoke made at once.
    document.body.appendChild(link)
    link.click()
    link.remove()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  const print = () => {
    const popup = window.open('', '_blank', 'width=480,height=640')
    if (!popup) return
    const pre = popup.document.createElement('pre')
    pre.textContent = codesText(codes, heading)
    popup.document.body.appendChild(pre)
    // print() does not block everywhere: the window closes once the dialog is done.
    popup.addEventListener('afterprint', () => popup.close())
    popup.print()
  }

  return (
    <div className="flex flex-col gap-4">
      <ol data-recovery-codes className="m-0 grid list-none grid-cols-2 gap-x-6 gap-y-1.5 rounded-md border border-line-soft bg-canvas p-4 font-mono text-sm text-ink">
        {codes.map((code) => (
          <li key={code}>{code}</li>
        ))}
      </ol>
      <div className="flex flex-wrap gap-2">
        <Button icon={<Copy size={14} aria-hidden="true" />} onClick={copy}>
          {t`Copy`}
        </Button>
        <Button icon={<Download size={14} aria-hidden="true" />} onClick={download}>
          {t`Download .txt`}
        </Button>
        <Button icon={<Printer size={14} aria-hidden="true" />} onClick={print}>
          {t`Print`}
        </Button>
      </div>
      <Checkbox checked={saved} onChange={(e) => onSavedChange(e.target.checked)}>
        {t`I saved my recovery codes somewhere safe`}
      </Checkbox>
    </div>
  )
}
