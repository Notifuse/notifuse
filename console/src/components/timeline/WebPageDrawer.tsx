import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Drawer, Button, Typography, Alert, Spin, Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faArrowUpRightFromSquare } from '@fortawesome/free-solid-svg-icons'

const { Text } = Typography

/**
 * How long to wait for the framed page before calling it blocked.
 *
 * There is no reliable event for "the browser refused to frame this": a page
 * served with X-Frame-Options or a CSP frame-ancestors directive is simply never
 * loaded, and Chrome fires nothing at all. So the only signal available is the
 * absence of `load` after a while.
 *
 * Firefox does fire `load` on its own error page, which reads here as a success
 * and leaves a blank frame. That is why the address and the open button sit
 * outside the frame and are always visible — the drawer stays useful whichever
 * way the heuristic falls.
 */
const IFRAME_LOAD_TIMEOUT_MS = 5000

interface WebPageDrawerProps {
  open: boolean
  url: string | null
  onClose: () => void
}

/**
 * Opens the page an identified visitor actually looked at.
 *
 * Most sites refuse to be framed, so this never relies on the frame succeeding:
 * the address and a way out to a real tab come first, and the preview is a bonus
 * underneath.
 */
export function WebPageDrawer({ open, url, onClose }: WebPageDrawerProps) {
  const { t } = useLingui()
  const [state, setState] = React.useState<'loading' | 'loaded' | 'blocked'>('loading')

  React.useEffect(() => {
    if (!open || !url) return

    setState('loading')
    const timer = setTimeout(
      () => setState((current) => (current === 'loading' ? 'blocked' : current)),
      IFRAME_LOAD_TIMEOUT_MS
    )
    return () => clearTimeout(timer)
  }, [open, url])

  return (
    <Drawer
      title={t`View website`}
      placement="right"
      // `size`, not `width`: antd v6 deprecated the latter, and it accepts a
      // percentage the same way the other drawers in the console use it.
      size="70%"
      open={open}
      onClose={onClose}
      styles={{ body: { display: 'flex', flexDirection: 'column', gap: 12, padding: 16 } }}
    >
      {url && (
        <>
          <div className="flex items-center gap-2">
            <Tooltip title={url}>
              <Text code ellipsis className="flex-1 min-w-0">
                {url}
              </Text>
            </Tooltip>
            <Button
              type="primary"
              href={url}
              target="_blank"
              rel="noreferrer noopener"
              icon={<FontAwesomeIcon icon={faArrowUpRightFromSquare} />}
            >
              {t`Open in new tab`}
            </Button>
          </div>

          {state === 'blocked' ? (
            <Alert
              type="info"
              showIcon
              message={t`This page cannot be previewed here`}
              description={t`The site refuses to be displayed inside another page, which is a common security setting. Open it in a new tab to see it.`}
            />
          ) : (
            <div className="relative flex-1 min-h-0 border border-gray-200 rounded overflow-hidden">
              {state === 'loading' && (
                <div className="absolute inset-0 flex items-center justify-center">
                  <Spin />
                </div>
              )}
              <iframe
                // Remounted per URL, so reopening on a different page does not
                // show the previous one while the new one loads.
                key={url}
                src={url}
                title={t`Website preview`}
                className="w-full h-full"
                // No allow-top-navigation: a framed page must not be able to
                // move the console out from under whoever opened it.
                sandbox="allow-scripts allow-same-origin allow-popups allow-forms"
                referrerPolicy="no-referrer"
                onLoad={() => setState('loaded')}
              />
            </div>
          )}
        </>
      )}
    </Drawer>
  )
}

export default WebPageDrawer
