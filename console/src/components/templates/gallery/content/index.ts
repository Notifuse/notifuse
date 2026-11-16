import type { GalleryLanguage } from '../types'
import { ca } from './ca'
import { de } from './de'
import { en, type GalleryContent } from './en'
import { es } from './es'
import { fr } from './fr'
import { it } from './it'
import { ja } from './ja'
import { ptBR } from './pt-BR'

export type { GalleryContent }

/** The gallery's content in each of its languages. */
export const GALLERY_CONTENT: Record<GalleryLanguage, GalleryContent> = {
  en,
  fr,
  es,
  de,
  ca,
  'pt-BR': ptBR,
  ja,
  it
}
