import { GALLERY_LANGUAGES, type GalleryLanguage } from './types'

/**
 * The language the gallery writes a workspace's emails in: the workspace's own when the gallery
 * has it, Brazilian Portuguese for Portuguese, English otherwise.
 */
export function resolveGalleryLanguage(language: string | undefined | null): GalleryLanguage {
  if (!language) return 'en'
  if ((GALLERY_LANGUAGES as readonly string[]).includes(language)) return language as GalleryLanguage
  const base = language.split('-')[0].toLowerCase()
  if (base === 'pt') return 'pt-BR'
  const match = GALLERY_LANGUAGES.find((candidate) => candidate === base)
  return match ?? 'en'
}
