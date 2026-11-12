import { useEffect, useState } from 'react'
import { Dropdown, Tooltip, type MenuProps } from 'antd'
import { Check, ChevronsUpDown, Languages, LogOut } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { useLocale } from '../../contexts/LocaleContext'
import { LANGUAGES, changeLanguage } from '../../i18n/languages'
import type { Locale } from '../../i18n'
import { getGravatarUrl, initialsFor } from './gravatar'

export type AccountRole = 'root' | 'owner' | 'member'

export interface AccountMenuProps {
  user: { email: string; name?: string } | null
  /** Unknown while the member loads. */
  role?: AccountRole
  collapsed: boolean
  onSignout: () => void
  getPopupContainer: () => HTMLElement
}

const LANGUAGE_PREFIX = 'language:'

/**
 * The Gravatar once it has loaded, null until then and after an error, so the initials show
 * instead of an empty circle.
 */
function useLoadedImage(src: string): string | null {
  const [loaded, setLoaded] = useState<string | null>(null)
  useEffect(() => {
    if (!src) return
    const image = new Image()
    image.onload = () => setLoaded(src)
    image.src = src
    return () => {
      image.onload = null
    }
  }, [src])
  return loaded === src ? src : null
}

function AccountAvatar({ user }: { user: AccountMenuProps['user'] }) {
  // 52px for a sharp 26px avatar on a dense screen.
  const photo = useLoadedImage(getGravatarUrl(user?.email, 52))
  return (
    <span
      data-account-avatar
      className="grid size-6.5 shrink-0 place-items-center overflow-hidden rounded-full bg-primary-100 text-2xs font-semibold text-primary-700"
    >
      {photo ? <img src={photo} alt="" className="size-full object-cover" /> : initialsFor(user)}
    </span>
  )
}

/** The account: avatar, email, role and language, with its menu opened above. */
export function AccountMenu({ user, role, collapsed, onSignout, getPopupContainer }: AccountMenuProps) {
  const { t } = useLingui()
  const { locale, setLocale } = useLocale()
  const [menuOpen, setMenuOpen] = useState(false)
  const roleLabel =
    role === 'root' ? t`Root` : role === 'owner' ? t`Owner` : role === 'member' ? t`Member` : null
  const email = user?.email ?? ''

  const items: MenuProps['items'] = [
    { key: 'email', label: email, disabled: true },
    {
      key: 'language',
      icon: <Languages size={14} aria-hidden="true" />,
      label: t`Language`,
      children: LANGUAGES.map((language) => ({
        key: `${LANGUAGE_PREFIX}${language.locale}`,
        label: (
          <span className="flex items-center justify-between gap-4">
            {language.name}
            {language.locale === locale ? <Check size={14} aria-label={t`Current language`} /> : null}
          </span>
        )
      }))
    },
    { type: 'divider' },
    { key: 'logout', icon: <LogOut size={14} aria-hidden="true" />, label: t`Logout` },
    { type: 'divider' },
    {
      key: 'version',
      disabled: true,
      label: <span className="font-mono text-overline text-ink-3">Notifuse v{window.VERSION || '1.0'}</span>
    }
  ]

  const handleClick: MenuProps['onClick'] = ({ key }) => {
    if (key === 'logout') onSignout()
    else if (key.startsWith(LANGUAGE_PREFIX))
      changeLanguage(setLocale, key.slice(LANGUAGE_PREFIX.length) as Locale)
  }

  const button = (
    <button
      type="button"
      aria-label={collapsed ? email : undefined}
      className={`flex cursor-pointer items-center gap-2 rounded-md border-0 bg-transparent p-1.5 text-left hover:bg-line-soft focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none ${
        collapsed ? 'mx-auto' : 'w-full'
      }`}
    >
      <AccountAvatar user={user} />
      {collapsed ? null : (
        <>
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="truncate text-sm font-semibold text-ink">{email}</span>
            <span data-account-role className="truncate text-xs text-ink-3">
              {roleLabel ? `${roleLabel} · ` : ''}
              {locale.toUpperCase()}
            </span>
          </span>
          <ChevronsUpDown size={14} aria-hidden="true" className="shrink-0 text-ink-3" />
        </>
      )}
    </button>
  )

  return (
    <Dropdown
      trigger={['click']}
      onOpenChange={setMenuOpen}
      placement={collapsed ? 'rightBottom' : 'topLeft'}
      getPopupContainer={getPopupContainer}
      // The language submenu opens in the sidebar too, so a click in it is not a click outside.
      menu={{ items, onClick: handleClick, getPopupContainer }}
    >
      {collapsed ? (
        <Tooltip
          title={email}
          placement="right"
          getPopupContainer={getPopupContainer}
          // A tooltip left over the menu it opened would cover its first or last row.
          open={menuOpen ? false : undefined}
        >
          {button}
        </Tooltip>
      ) : (
        button
      )}
    </Dropdown>
  )
}
