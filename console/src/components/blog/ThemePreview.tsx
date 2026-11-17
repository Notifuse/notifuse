import { useState, useEffect } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Spin, Alert } from 'antd'
import { BlogCategory, BlogThemeFiles } from '../../services/api/blog'
import { renderBlogPage, RenderResult } from '../../utils/liquidRenderer'
import { getMockDataForView } from '../../utils/mockBlogData'
import { withPreviewStorage } from '../../utils/previewStorage'
import { Workspace } from '../../services/api/types'
import { withWorkspaceData } from './themePreviewData'

interface ThemePreviewProps {
  files: BlogThemeFiles
  workspace?: Workspace | null
  /** The blog's categories, shown in place of the sample ones. */
  categories?: BlogCategory[]
  view: ViewType
}

type ViewType = 'home' | 'category' | 'post'

export function ThemePreview({ files, workspace, categories, view }: ThemePreviewProps) {
  const { t } = useLingui()
  const [renderResult, setRenderResult] = useState<RenderResult | null>(null)
  const [isRendering, setIsRendering] = useState(false)

  useEffect(() => {
    const renderPreview = async () => {
      setIsRendering(true)
      try {
        // Sample posts, in the workspace's own name, address, settings and categories.
        const mockData = withWorkspaceData(getMockDataForView(view), workspace, categories, window.API_ENDPOINT)

        const result = await renderBlogPage(files, view, mockData)
        setRenderResult(result)
      } catch (error) {
        console.error('Preview rendering failed:', error)
        setRenderResult({
          success: false,
          error: t`Failed to render preview`
        })
      } finally {
        setIsRendering(false)
      }
    }

    renderPreview()
  }, [files, view, workspace, categories, t])

  return (
    <div
      style={{
        height: '100%',
        overflow: 'auto',
        background: '#ffffff',
        position: 'relative'
      }}
    >
      {isRendering && (
        <div
          style={{
            position: 'absolute',
            top: '50%',
            left: '50%',
            transform: 'translate(-50%, -50%)',
            zIndex: 10
          }}
        >
          <Spin size="large" description={t`Rendering preview...`} />
        </div>
      )}

      {!isRendering && renderResult && !renderResult.success && (
        <div style={{ padding: 24 }}>
          <Alert
            title={t`Template Error`}
            description={
              <div>
                <p>{renderResult.error}</p>
                {renderResult.errorLine && <p>{t`Line`}: {renderResult.errorLine}</p>}
              </div>
            }
            type="error"
            showIcon
          />
        </div>
      )}

      {!isRendering && renderResult && renderResult.success && renderResult.html && (
        <iframe
          srcDoc={withPreviewStorage(renderResult.html)}
          style={{
            width: '100%',
            height: '100%',
            border: 'none',
            background: '#ffffff'
          }}
          title="Blog Preview"
          // Theme scripts run, in an origin of their own rather than the console's.
          sandbox="allow-scripts"
        />
      )}
    </div>
  )
}
