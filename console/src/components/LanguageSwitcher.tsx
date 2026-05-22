import { Dropdown, Button } from 'antd'
import { useLocale } from '../contexts/LocaleContext'
import { authService } from '../services/api/auth'
import type { Locale } from '../i18n'
import type { MenuProps } from 'antd'

export function LanguageSwitcher() {
  const { locale, setLocale, locales, localeNames } = useLocale()

  const handleSelect = (l: Locale) => {
    void setLocale(l)
    // Persist the choice to the user's profile so it drives both the console
    // UI and the system emails sent to them. Fire-and-forget: a failed save
    // must not block the UI locale change.
    authService.updateLanguage(l).catch((err) => {
      console.error('Failed to persist language preference:', err)
    })
  }

  const items: MenuProps['items'] = locales.map((l) => ({
    key: l,
    label: localeNames[l],
    onClick: () => handleSelect(l)
  }))

  return (
    <Dropdown trigger={['click']} menu={{ items, selectedKeys: [locale] }} placement="bottomRight">
      <Button color="default" variant="filled">
        {locale.toUpperCase()}
      </Button>
    </Dropdown>
  )
}
