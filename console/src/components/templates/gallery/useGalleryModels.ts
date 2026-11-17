import { useMemo } from 'react'
import type { Template } from '../../../services/api/template'
import type { Workspace } from '../../../services/api/types'
import { buildStartingPoint, galleryAssets, type GalleryStart } from './buildStartingPoint'
import { loadLogoWidth, useLogoWidth } from './logoWidth'
import type { GalleryAssets, GalleryModel } from './types'

/**
 * The gallery's images, served by this instance's console. In production the instance serves the
 * console, so the API endpoint (the address recipients reach) serves the images. The dev server
 * serves them itself: the API does not have them.
 */
export function useGalleryAssets(): GalleryAssets {
  return useMemo(
    () => galleryAssets(import.meta.env.DEV ? window.location.origin : window.API_ENDPOINT || window.location.origin),
    []
  )
}

export interface GalleryModels {
  /** A model built for the workspace, to start a template from; waits for the logo's size. */
  start: (model: GalleryModel) => Promise<GalleryStart>
  /** A model as a template a preview can compile, in the workspace's language. */
  previewTemplate: (model: GalleryModel) => Template
  /** Whether the logo's size is known: a preview compiled before would be compiled twice. */
  logoReady: boolean
}

/** The gallery's models for a workspace: its languages, its name, its logo sized once loaded. */
export function useGalleryModels(
  workspace: Workspace,
  assets: GalleryAssets,
  defaultTestData: Record<string, unknown>
): GalleryModels {
  const logoUrl = workspace.settings.logo_url || undefined
  const logoWidth = useLogoWidth(logoUrl)
  const workspaceLanguages = {
    defaultLanguage: workspace.settings.default_language,
    languages: workspace.settings.languages,
    name: workspace.name,
    logoUrl,
    logoWidth
  }

  // A pick made before the logo has loaded waits for its size: built without it, the logo
  // would keep a square's width in the saved template.
  const start = async (model: GalleryModel) => {
    const width = logoUrl ? (logoWidth ?? (await loadLogoWidth(logoUrl))) : undefined
    return buildStartingPoint(model, { ...workspaceLanguages, logoWidth: width }, assets, defaultTestData)
  }

  const previewTemplate = useMemo(() => {
    const cache = new Map<string, Template>()
    return (model: GalleryModel): Template => {
      const cached = cache.get(model.id)
      if (cached) return cached
      const built = buildStartingPoint(model, workspaceLanguages, assets, defaultTestData)
      const template = {
        // The logo's width is in the id: the compiled preview is cached by it.
        id: `gallery-${model.id}-${built.language}-${logoWidth ?? 'logo'}`,
        name: built.name,
        version: 1,
        channel: 'email',
        category: model.category ?? '',
        email: { subject: built.subject, subject_preview: built.subjectPreview, editor_mode: 'visual', visual_editor_tree: built.tree },
        test_data: built.testData
      } as unknown as Template
      cache.set(model.id, template)
      return template
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- the workspace's languages and the assets are fixed while the gallery is shown
  }, [workspace.id, assets.baseUrl, logoWidth])

  return { start, previewTemplate, logoReady: !logoUrl || logoWidth !== undefined }
}
