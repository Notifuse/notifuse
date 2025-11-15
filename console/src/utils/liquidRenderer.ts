import { BlogThemeFiles } from '../services/api/blog'
import { MockBlogData } from './mockBlogData'
import { createSecureLiquidEngine, validateTemplateSecurity } from './liquidConfig'

const liquid = createSecureLiquidEngine()

export interface RenderResult {
  success: boolean
  html?: string
  error?: string
  errorLine?: number
}

/**
 * Render a complete blog page using Liquid templates
 */
export async function renderBlogPage(
  files: BlogThemeFiles,
  view: 'home' | 'category' | 'post',
  data: MockBlogData
): Promise<RenderResult> {
  try {
    // Combine templates: shared macros + header + selected view + footer
    const template = `
      ${files.shared}
      ${files.header}
      ${files[view]}
      ${files.footer}
    `

    // SECURITY: Validate template before rendering
    const securityIssues = validateTemplateSecurity(template)
    if (securityIssues.length > 0) {
      return {
        success: false,
        error: `Security validation failed: ${securityIssues.join(', ')}`
      }
    }

    // Prepare data based on view
    let renderData = { ...data }

    if (view === 'category') {
      renderData = {
        ...data,
        category: data.currentCategory || data.categories[0]
      }
    }

    if (view === 'post') {
      renderData = {
        ...data,
        post: data.currentPost || data.posts[0],
        previous_post: data.previous_post,
        next_post: data.next_post
      }
    }

    // SECURITY: Timeout protection
    const timeoutPromise = new Promise<never>((_, reject) => {
      setTimeout(() => reject(new Error('Template execution timeout (5s limit)')), 5000)
    })

    // Render with Liquid
    const renderPromise = liquid.parseAndRender(template, renderData)
    const html = await Promise.race([renderPromise, timeoutPromise])

    return {
      success: true,
      html
    }
  } catch (error: any) {
    console.error('Liquid rendering error:', error)

    // Try to extract line number from error
    let errorLine: number | undefined
    const lineMatch = error.message?.match(/line (\d+)/i)
    if (lineMatch) {
      errorLine = parseInt(lineMatch[1], 10)
    }

    return {
      success: false,
      error: error.message || 'Failed to render template',
      errorLine
    }
  }
}

/**
 * Validate Liquid syntax without rendering
 */
export async function validateLiquidSyntax(template: string): Promise<RenderResult> {
  try {
    await liquid.parse(template)
    return { success: true }
  } catch (error: any) {
    let errorLine: number | undefined
    const lineMatch = error.message?.match(/line (\d+)/i)
    if (lineMatch) {
      errorLine = parseInt(lineMatch[1], 10)
    }

    return {
      success: false,
      error: error.message || 'Syntax error',
      errorLine
    }
  }
}

