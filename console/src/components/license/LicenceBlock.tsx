import type { ReactNode } from 'react'
import { Button } from 'antd'
import { useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'

interface LicenceBlockProps {
  title: string
  description: ReactNode
  /** The workspace the block is shown in, which is where its one button routes. */
  workspaceId?: string
  className?: string
}

// The same gradient BulkActionsBar draws its border with, anchored on the primary so the block
// reads as the product's own voice rather than as a warning: this is an offer, not an alarm.
const GRADIENT = 'bg-gradient-to-r from-primary via-fuchsia-500 to-pink-500'

/**
 * The one shape every licence message in the console takes: a gradient-bordered block with a
 * title, a sentence on what still works, and a single button to Settings › Licence.
 *
 * One button, for everyone. That page already offers root the key box and the price list, and
 * already tells a member who to ask — so the block repeats neither, and never shows "Buy" to
 * someone who cannot install what they would buy.
 *
 * Presentational only: what to say, and whether to say it, is the caller's — LicenceGateNotice
 * for a locked control, SsoLicenceNotice for the one capability that has no control to lock.
 */
export function LicenceBlock({
  title,
  description,
  workspaceId,
  className = 'mb-4'
}: LicenceBlockProps) {
  const { t } = useLingui()
  const navigate = useNavigate()

  const openLicenseSettings = () => {
    if (!workspaceId) return
    navigate({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId, section: 'licence' }
    })
  }

  return (
    // A gradient border is a gradient box with a white box inside it; the padding is the
    // border's width. Drawn this way rather than with border-image so the corners stay round.
    <div role="note" className={`${GRADIENT} rounded-lg p-[1.5px] ${className}`}>
      <div className="rounded-[6.5px] bg-white px-5 py-4">
        <div className="min-w-0">
          <div
            className={`${GRADIENT} mb-1 inline-block bg-clip-text text-[11px] font-semibold uppercase tracking-wider text-transparent`}
          >
            {t`Licensed capability`}
          </div>
          <div className="text-sm font-semibold text-gray-900">{title}</div>
          <div className="mt-1 text-sm text-gray-600">{description}</div>

          {workspaceId && (
            <div className="mt-3">
              <Button size="small" type="primary" onClick={openLicenseSettings}>
                {t`Licence settings`}
              </Button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
