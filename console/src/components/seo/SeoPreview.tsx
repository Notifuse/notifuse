import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Segmented } from 'antd'
import { Eye, ImageOff } from 'lucide-react'
import { Panel } from '../layout'

/** What a page shows to search engines and on social networks, after its fallbacks. */
export interface EffectiveSeo {
  title: string
  description: string
  ogTitle: string
  ogDescription: string
  ogImage: string
}

export interface SeoPreviewProps {
  effective: EffectiveSeo
  /** The blog's address without the protocol (blog.acme.com). */
  domain: string
  /** The page's path segments after the domain (a category's slug, then a post's). */
  path: string[]
  /** The blog's icon, beside the address as a search result shows it. */
  iconUrl?: string
}

type Surface = 'search' | 'social'

/** A blog page as a search result and as a shared link, from the tags it will carry. */
export function SeoPreview({ effective, domain, path, iconUrl }: SeoPreviewProps) {
  const { t } = useLingui()
  const [surface, setSurface] = useState<Surface>('search')
  const address = [domain, ...path.filter(Boolean)].join(' › ')

  return (
    <Panel
      title={t`Preview`}
      icon={<Eye size={14} aria-hidden="true" />}
      extra={
        <Segmented<Surface>
          size="small"
          value={surface}
          onChange={setSurface}
          options={[
            { value: 'search', label: t`Search` },
            { value: 'social', label: t`Social` }
          ]}
        />
      }
    >
      {surface === 'search' ? (
        <div data-preview="search" className="flex flex-col gap-0.5">
          <div className="flex min-w-0 items-center gap-2 text-xs text-ink-2">
            {iconUrl ? (
              <img src={iconUrl} alt="" className="size-4 shrink-0 rounded-full object-cover" />
            ) : (
              <span aria-hidden="true" className="size-4 shrink-0 rounded-full bg-fill" />
            )}
            <span className="truncate">{address}</span>
          </div>
          <div className="line-clamp-2 text-base text-info-fg">{effective.title}</div>
          {effective.description ? (
            <div className="line-clamp-3 text-xs text-ink-2">{effective.description}</div>
          ) : (
            <div className="text-xs text-ink-3 italic">{t`Search engines pick a sentence from the page`}</div>
          )}
        </div>
      ) : (
        <div data-preview="social" className="overflow-hidden rounded-md border border-line">
          {effective.ogImage ? (
            <img src={effective.ogImage} alt="" className="aspect-[1.91/1] w-full object-cover" />
          ) : (
            <div className="flex aspect-[1.91/1] w-full flex-col items-center justify-center gap-1 bg-fill text-xs text-ink-3">
              <ImageOff size={16} aria-hidden="true" />
              {t`No share image`}
            </div>
          )}
          <div className="flex flex-col gap-0.5 border-t border-line bg-subtle px-3 py-2">
            <span className="truncate text-2xs tracking-wide text-ink-3 uppercase">{domain}</span>
            <span className="truncate text-sm font-semibold text-ink">{effective.ogTitle}</span>
            {effective.ogDescription ? (
              <span className="line-clamp-2 text-xs text-ink-2">{effective.ogDescription}</span>
            ) : null}
          </div>
        </div>
      )}
    </Panel>
  )
}
