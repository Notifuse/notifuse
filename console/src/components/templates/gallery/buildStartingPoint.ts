import type { EmailBlock } from '../../email_builder/types'
import type { TemplateTranslation } from '../../../services/api/template'
import { GALLERY_CONTENT } from './content'
import { resolveGalleryLanguage } from './resolveGalleryLanguage'
import type { GalleryAssets, GalleryModel } from './types'

/** What a new template starts with when made from a gallery model. */
export interface GalleryStart {
  modelId: string
  name: string
  category?: string
  subject: string
  subjectPreview: string
  tree: EmailBlock
  testData: Record<string, unknown>
  /** The other languages of the workspace the gallery is written in, ready as translations. */
  translations: Record<string, TemplateTranslation>
  /** The language the default content is written in. */
  language: string
}

/** The address of the gallery's images on an instance: the console serves them. */
export function galleryAssets(apiEndpoint: string): GalleryAssets {
  return { baseUrl: `${apiEndpoint.replace(/\/+$/, '')}/console/gallery` }
}

/**
 * Builds a gallery model for a workspace: in its default language, with a translation for each
 * of its other languages the gallery has. The model's test data goes over the template's
 * default data, so its variables render in the preview.
 */
export function buildStartingPoint(
  model: GalleryModel,
  workspace: { defaultLanguage?: string; languages?: string[]; name?: string; logoUrl?: string; logoWidth?: number },
  baseAssets: GalleryAssets,
  defaultTestData: Record<string, unknown>
): GalleryStart {
  const language = resolveGalleryLanguage(workspace.defaultLanguage)
  const assets: GalleryAssets = {
    ...baseAssets,
    logoUrl: workspace.logoUrl || baseAssets.logoUrl,
    logoWidth: workspace.logoWidth ?? baseAssets.logoWidth
  }
  const content = (code: keyof typeof GALLERY_CONTENT) => withBrand(GALLERY_CONTENT[code], workspace.name)
  const main = model.build(content(language), assets)

  const translations: Record<string, TemplateTranslation> = {}
  for (const workspaceLanguage of workspace.languages ?? []) {
    if (workspaceLanguage === workspace.defaultLanguage) continue
    const translated = resolveGalleryLanguage(workspaceLanguage)
    // A language the gallery lacks would only get the English text again: left to the author.
    if (translated === 'en' && !workspaceLanguage.toLowerCase().startsWith('en')) continue
    if (translated === language) continue
    const built = model.build(content(translated), assets)
    translations[workspaceLanguage] = {
      email: {
        subject: built.subject,
        subject_preview: built.preheader,
        editor_mode: 'visual',
        visual_editor_tree: built.tree as unknown as NonNullable<TemplateTranslation['email']>['visual_editor_tree']
      } as TemplateTranslation['email']
    }
  }

  return {
    modelId: model.id,
    name: content(language).models[model.id].name,
    category: model.category,
    subject: main.subject,
    subjectPreview: main.preheader,
    tree: main.tree,
    testData: { ...defaultTestData, ...main.testData },
    translations,
    language
  }
}

// The example brand the gallery is written with, replaced by the workspace's name.
const EXAMPLE_BRAND = /\bAcme\b/g

/**
 * The gallery's content with the workspace's name instead of « Acme » (signature, footer, subjects).
 * The name loses what HTML or Liquid would read (< > { }): it lands in mj-text HTML and in
 * subjects rendered by Liquid. Without a name, the example stays.
 */
export function withBrand<T>(content: T, name: string | undefined): T {
  const brand = (name ?? '').replace(/[<>{}]/g, '').trim()
  if (!brand) return content
  const map = (value: unknown): unknown => {
    if (typeof value === 'string') return value.replace(EXAMPLE_BRAND, brand)
    if (value && typeof value === 'object') {
      return Object.fromEntries(Object.entries(value as Record<string, unknown>).map(([key, child]) => [key, map(child)]))
    }
    return value
  }
  return map(content) as T
}
