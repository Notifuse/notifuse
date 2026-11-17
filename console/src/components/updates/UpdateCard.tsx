import { Button, Popconfirm } from 'antd'
import { CloseOutlined, SafetyCertificateOutlined, WarningOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import type { UpdateCheckResponse } from '../../services/updates'

interface UpdateCardProps {
  data: UpdateCheckResponse
  onOpen: () => void
  onIgnore: (version: string) => void
}

const CARD_TONES = {
  normal: 'border-primary-300 bg-primary-50',
  security: 'border-danger-fg/30 bg-danger-bg'
} as const

/**
 * The announcement at the bottom of the workspace list: the latest release, how far behind
 * this server is, the newest release's first highlights and the warnings to read before
 * upgrading. A release that fixes a vulnerability turns it red and takes away the way to
 * dismiss it.
 */
export function UpdateCard({ data, onOpen, onIgnore }: UpdateCardProps) {
  const { t } = useLingui()
  const latest = data.latest
  const highlights = (data.releases[0]?.highlights ?? []).slice(0, 2)
  const warnings = data.releases.reduce((count, release) => count + release.before_upgrade.length, 0)
  const securityRelease = data.releases.find((release) => release.security)
  const securityVersion = securityRelease?.version ?? ''
  const behind = plural(data.behind, { one: '# version behind', other: '# versions behind' })
  const toRead = plural(warnings, {
    one: '# point to read before upgrading',
    other: '# points to read before upgrading'
  })

  return (
    <section
      aria-label={t`Update available`}
      className={`relative flex flex-col gap-1 rounded-lg border p-3 ${securityRelease ? CARD_TONES.security : CARD_TONES.normal}`}
    >
      {!securityRelease && (
        <div className="absolute right-1.5 top-1.5">
          <Popconfirm
            title={t`Ignore ${latest}?`}
            description={t`The card will come back with the next version. This choice applies to this browser.`}
            okText={t`Ignore`}
            cancelText={t`Cancel`}
            onConfirm={() => onIgnore(latest)}
          >
            <Button type="text" size="small" icon={<CloseOutlined />} aria-label={t`Ignore this version`} />
          </Popconfirm>
        </div>
      )}
      <div className="pr-6 text-base font-semibold text-ink">{t`Notifuse ${latest} is available`}</div>
      <div className="text-xs text-ink-2">{behind}</div>
      {securityRelease && (
        <div className="mt-1 flex items-center gap-1.5 text-xs font-medium text-danger-fg">
          <SafetyCertificateOutlined aria-hidden />
          {t`Includes a security fix (${securityVersion})`}
        </div>
      )}
      {!securityRelease && highlights.length > 0 && (
        <ul className="my-1 list-disc pl-4 text-xs text-ink-2">
          {highlights.map((highlight, index) => (
            <li key={index}>{highlight}</li>
          ))}
        </ul>
      )}
      {warnings > 0 && (
        <div className="flex items-center gap-1.5 text-xs text-warning-fg">
          <WarningOutlined aria-hidden />
          {toRead}
        </div>
      )}
      <div className="mt-2">
        <Button type="primary" danger={!!securityRelease} block onClick={onOpen}>
          {t`See what's new`}
        </Button>
      </div>
    </section>
  )
}
