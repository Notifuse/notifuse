import { Liquid } from 'liquidjs'
import { BlogThemeFiles } from '../services/api/blog'
import { MockBlogData } from './mockBlogData'
import { validateTemplateSecurity } from './liquidConfig'

export interface RenderResult {
  success: boolean
  html?: string
  error?: string
  errorLine?: number
}

/**
 * Create a Liquid engine with registered templates for includes
 */
function createLiquidEngineWithTemplates(files: BlogThemeFiles): Liquid {
  const liquid = new Liquid({
    // SECURITY: Throw errors on undefined filters
    strictFilters: true,
    // SECURITY: Don't throw on undefined variables
    strictVariables: false,
    // SECURITY: Explicit timezone
    timezoneOffset: 0,
    // Allow lenient if statements
    lenientIf: true,
    // Preserve whitespace
    trimTagRight: false,
    trimTagLeft: false,
    trimOutputRight: false,
    trimOutputLeft: false,
    // Greedy mode for better performance
    greedy: true,
    // Enable file system support for includes
    fs: {
      readFileSync: (file: string) => {
        // Remove .liquid extension if present
        const fileName = file.replace(/\.liquid$/, '')

        // Map template names to files
        const templateMap: Record<string, string> = {
          shared: files['shared.liquid'] || '',
          header: files['header.liquid'] || '',
          footer: files['footer.liquid'] || '',
          home: files['home.liquid'] || '',
          category: files['category.liquid'] || '',
          post: files['post.liquid'] || ''
        }

        if (templateMap[fileName] !== undefined) {
          return templateMap[fileName]
        }

        throw new Error(`Template not found: ${file}`)
      },
      readFile: async (file: string) => {
        // Same as sync version for async calls
        const fileName = file.replace(/\.liquid$/, '')

        const templateMap: Record<string, string> = {
          shared: files['shared.liquid'] || '',
          header: files['header.liquid'] || '',
          footer: files['footer.liquid'] || '',
          home: files['home.liquid'] || '',
          category: files['category.liquid'] || '',
          post: files['post.liquid'] || ''
        }

        if (templateMap[fileName] !== undefined) {
          return templateMap[fileName]
        }

        throw new Error(`Template not found: ${file}`)
      },
      existsSync: (file: string) => {
        const fileName = file.replace(/\.liquid$/, '')
        return ['shared', 'header', 'footer', 'home', 'category', 'post'].includes(fileName)
      },
      exists: async (file: string) => {
        const fileName = file.replace(/\.liquid$/, '')
        return ['shared', 'header', 'footer', 'home', 'category', 'post'].includes(fileName)
      },
      resolve: (_root: string, file: string, _ext: string) => {
        return file
      }
    } as any // Type assertion needed due to incomplete TypeScript definitions
  })

  return liquid
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
    // Create a Liquid engine with registered templates
    const liquid = createLiquidEngineWithTemplates(files)

    // Inline styles into header (replace the comment with actual styles)
    const headerWithStyles = files['header.liquid'].replace(
      '<!-- Styles will be inlined here by the backend -->',
      `<style>${files['styles.css']}</style>`
    )

    // Determine which view file to use
    const viewKey = `${view}.liquid` as keyof BlogThemeFiles

    // Combine templates: header + selected view + footer
    // Note: shared.liquid is available for {% include 'shared' %} via the file system
    const template = `
      ${headerWithStyles}
      ${files[viewKey]}
      ${files['footer.liquid']}
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
    // Note: MockBlogData already includes post and category fields set by getMockDataForView
    // This matches the backend BlogTemplateDataRequest structure
    let renderData = { ...data }

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
export async function validateLiquidSyntax(
  template: string,
  files?: BlogThemeFiles
): Promise<RenderResult> {
  try {
    // If files are provided, create engine with includes support
    let liquid: Liquid
    if (files) {
      liquid = createLiquidEngineWithTemplates(files)
    } else {
      // Fallback to basic engine without includes
      liquid = new Liquid({
        strictFilters: true,
        strictVariables: false,
        timezoneOffset: 0,
        lenientIf: true,
        trimTagRight: false,
        trimTagLeft: false,
        trimOutputRight: false,
        trimOutputLeft: false,
        greedy: true
      })
    }

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
