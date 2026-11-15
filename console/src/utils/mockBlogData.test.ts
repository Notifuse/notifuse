import { describe, it, expect } from 'vitest'
import { readFileSync } from 'fs'
import { resolve } from 'path'
import { getMockDataForView } from './mockBlogData'

describe('mock blog data', () => {
  it('shows no photo of a person: every image is one shipped with the console', () => {
    const source = readFileSync(resolve(__dirname, 'mockBlogData.ts'), 'utf8')
    expect(source).not.toMatch(/https?:\/\/[^'"\s)]*\.(?:jpe?g|png|webp)|images\.unsplash\.com/)
    const data = JSON.stringify(['home', 'category', 'post'].map((view) => getMockDataForView(view as 'home')))
    const urls = data.match(/"(?:featured_image_url|avatar_url|og_image)":"[^"]*"/g) ?? []
    expect(urls.length).toBeGreaterThan(0)
    // Vite inlines the two small SVGs, or serves them from the bundle
    for (const url of urls) expect(url).toMatch(/":"(?:data:image\/svg\+xml|[^"]*blog-preview\/(?:landscape|avatar)[^"]*\.svg)/)
  })
})
