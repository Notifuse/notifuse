import { describe, expect, it } from 'vitest'
import { renderHook } from '@testing-library/react'
import type { Workspace } from '../../../services/api/types'
import { galleryModel } from './models'
import { useGalleryAssets, useGalleryModels } from './useGalleryModels'

const workspace = { id: 'ws1', name: 'Acme', settings: { default_language: 'fr', languages: ['fr', 'en'] } } as unknown as Workspace
const assets = { baseUrl: 'https://api.example.com/console/gallery' }

describe('useGalleryAssets', () => {
  it("serves the gallery's images from the console", () => {
    const { result } = renderHook(() => useGalleryAssets())
    expect(result.current.baseUrl).toMatch(/\/console\/gallery$/)
  })
})

describe('useGalleryModels', () => {
  it("builds a model in the workspace's language, with its name and the test data on top of the default", async () => {
    const { result } = renderHook(() => useGalleryModels(workspace, assets, { unsubscribe_url: 'https://u' }))
    const start = await result.current.start(galleryModel('order')!)
    expect(start.modelId).toBe('order')
    expect(start.language).toBe('fr')
    expect(start.category).toBe('transactional')
    expect(Object.keys(start.translations)).toEqual(['en'])
    expect(start.testData.unsubscribe_url).toBe('https://u')
    expect(start.testData.order).toBeDefined()
    expect(JSON.stringify(start.tree)).toContain('Acme')
  })

  it('gives each model one preview template, its id naming the model and the language', () => {
    const { result } = renderHook(() => useGalleryModels(workspace, assets, {}))
    const model = galleryModel('confirm')!
    const preview = result.current.previewTemplate(model)
    expect(preview.id).toBe('gallery-confirm-fr-logo')
    expect(preview.category).toBe('opt_in')
    expect(preview.email?.visual_editor_tree).toBeTruthy()
    // Cached: the same object, so its compiled preview is not asked for again.
    expect(result.current.previewTemplate(model)).toBe(preview)
  })

  it('is ready at once without a logo to measure', () => {
    const { result } = renderHook(() => useGalleryModels(workspace, assets, {}))
    expect(result.current.logoReady).toBe(true)
  })
})
