import type { EmailBlock } from '../../email_builder/types'
import type { GalleryContent } from './content/en'

/** The languages the gallery is written in: the console's. */
export const GALLERY_LANGUAGES = ['en', 'fr', 'es', 'de', 'ca', 'pt-BR', 'ja', 'it'] as const
export type GalleryLanguage = (typeof GALLERY_LANGUAGES)[number]

/** The filters of the gallery, by what the email is for rather than by category. */
export const GALLERY_USE_CASES = ['account', 'billing', 'shop', 'news', 'lifecycle'] as const
export type GalleryUseCase = (typeof GALLERY_USE_CASES)[number]

export type GalleryModelId = keyof GalleryContent['models']

/** What sends an email made from a model, as its preview says. */
export type GallerySender = 'any' | 'api' | 'broadcast' | 'automation' | 'list'

export interface GalleryAssets {
  /** Where the gallery's images are served: the instance's console, `${API_ENDPOINT}/console/gallery`. */
  baseUrl: string
  /** The workspace's own logo, used instead of the gallery's placeholder when it has one. */
  logoUrl?: string
  /** That logo's width at the headers' 40px height, once its natural size is known. */
  logoWidth?: number
}

/** A model, built in one language. */
export interface GalleryEmail {
  subject: string
  preheader: string
  tree: EmailBlock
  testData: Record<string, unknown>
}

export interface GalleryModel {
  id: GalleryModelId
  /** None for Blank, which is first and in every filter. */
  useCase?: GalleryUseCase
  /** The category the template is created with; none for Blank, chosen in Settings. */
  category?: string
  sentBy: GallerySender
  build: (content: GalleryContent, assets: GalleryAssets) => GalleryEmail
}
