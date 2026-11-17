import { Button } from 'antd'
import { ExportOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { SidePanel } from '../layout/SidePanel'
import { safeHttpsUrl, type UpdateCheckResponse, type UpdateRelease } from '../../services/updates'

interface UpdateDrawerProps {
  open: boolean
  onClose: () => void
  data: UpdateCheckResponse
}

/** The missing releases, newest first, with what each brings and what to do before upgrading. */
export function UpdateDrawer({ open, onClose, data }: UpdateDrawerProps) {
  const { t } = useLingui()
  const current = data.current
  const latest = data.latest
  const upgradeUrl = safeHttpsUrl(data.upgrade_url)
  const count = plural(data.behind, { one: '# version', other: '# versions' })

  return (
    <SidePanel
      open={open}
      onClose={onClose}
      title={t`What's new since ${current}`}
      meta={[`${current} → ${latest}`, count]}
      copyLink={false}
      footer={
        <div className="flex justify-end">
          <Button onClick={onClose}>{t`Close`}</Button>
        </div>
      }
    >
      {upgradeUrl && (
        <a href={upgradeUrl} target="_blank" rel="noopener noreferrer" className="text-xs">
          {t`How to upgrade`} <ExportOutlined aria-hidden />
        </a>
      )}
      <div className="flex flex-col divide-y divide-line-soft">
        {data.releases.map((release) => (
          <ReleaseSection key={release.version} release={release} />
        ))}
      </div>
    </SidePanel>
  )
}

function ReleaseSection({ release }: { release: UpdateRelease }) {
  const { t, i18n } = useLingui()
  const changelogUrl = safeHttpsUrl(release.changelog_url)
  const warnings = release.before_upgrade.length
  const warningCount = plural(warnings, { one: '# warning', other: '# warnings' })
  const date = formatReleaseDate(release.released_at, i18n.locale)

  return (
    <section className="flex flex-col gap-1.5 py-3" aria-label={release.version}>
      <div className="flex items-baseline gap-2">
        <span className="text-subtitle font-bold text-ink">{release.version}</span>
        <span className="text-xs text-ink-3">{date}</span>
        {release.security && (
          <span className="rounded-full bg-danger-bg px-2 text-overline font-semibold text-danger-fg">
            {t`Security`}
          </span>
        )}
        {warnings > 0 && (
          <span className="rounded-full bg-warning-bg px-2 text-overline font-semibold text-warning-fg">
            {warningCount}
          </span>
        )}
      </div>
      <p className="m-0 text-xs text-ink-2">{release.summary}</p>
      {release.highlights.length > 0 && (
        <ul className="m-0 list-disc pl-4 text-xs text-ink-2">
          {release.highlights.map((highlight, index) => (
            <li key={index}>{highlight}</li>
          ))}
        </ul>
      )}
      {release.security && release.security_note && (
        <div className="rounded-md bg-danger-bg px-2.5 py-2 text-xs text-danger-fg">
          <strong>{t`Security:`}</strong> {release.security_note}
        </div>
      )}
      {release.before_upgrade.map((warning, index) => (
        <div key={index} className="rounded-md bg-warning-bg px-2.5 py-2 text-xs text-warning-fg">
          <strong>{t`Before you upgrade:`}</strong> {warning}
        </div>
      ))}
      {changelogUrl && (
        <a href={changelogUrl} target="_blank" rel="noopener noreferrer" className="text-xs">
          {t`Full changelog`} <ExportOutlined aria-hidden />
        </a>
      )}
    </section>
  )
}

/** "2026-11-12" in the console's language; the raw date if it does not parse. */
function formatReleaseDate(isoDate: string, locale: string): string {
  const date = new Date(`${isoDate}T00:00:00Z`)
  if (Number.isNaN(date.getTime())) return isoDate
  try {
    return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(date)
  } catch {
    return isoDate
  }
}
