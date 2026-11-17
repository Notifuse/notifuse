import { useMemo } from 'react'
import { useLingui } from '@lingui/react/macro'
import { SUPPORTED_LANGUAGES } from '../../lib/languages'

/** A language's name in the console's language (« français » → « Français »), else its English name. */
export function useLanguageName(): (code: string) => string {
  const { i18n } = useLingui()
  const displayNames = useMemo(() => {
    try {
      return new Intl.DisplayNames([i18n.locale], { type: 'language' })
    } catch {
      return undefined
    }
  }, [i18n.locale])
  return (code: string) => {
    try {
      const name = displayNames?.of(code)
      if (name && name !== code) return name.charAt(0).toLocaleUpperCase(i18n.locale) + name.slice(1)
    } catch {
      // An unknown locale or code: the English name below
    }
    return SUPPORTED_LANGUAGES[code] || code
  }
}
