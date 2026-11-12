import { Dropdown, Button } from 'antd'
import { useLocale } from '../contexts/LocaleContext'
import { LANGUAGES, changeLanguage } from '../i18n/languages'
import type { MenuProps } from 'antd'

export function LanguageSwitcher() {
  const { locale, setLocale } = useLocale()

  const items: MenuProps['items'] = LANGUAGES.map(({ locale: l, name }) => ({
    key: l,
    label: name,
    onClick: () => changeLanguage(setLocale, l)
  }))

  return (
    <Dropdown trigger={['click']} menu={{ items, selectedKeys: [locale] }} placement="bottomRight">
      <Button color="default" variant="filled">
        {locale.toUpperCase()}
      </Button>
    </Dropdown>
  )
}
