import { Dropdown, Button } from 'antd'
import { useLocale } from '../contexts/LocaleContext'
import type { MenuProps } from 'antd'

export function LanguageSwitcher() {
  const { locale, setLocale, locales, localeNames } = useLocale()

  const items: MenuProps['items'] = locales.map((l) => ({
    key: l,
    label: localeNames[l],
    onClick: () => setLocale(l),
  }))

  return (
    <Dropdown
      trigger={['click']}
      menu={{ items, selectedKeys: [locale] }}
      placement="bottomRight"
    >
      <Button color="default" variant="filled">
        {locale.toUpperCase()}
      </Button>
    </Dropdown>
  )
}
