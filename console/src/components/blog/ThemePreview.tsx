import { useState, useEffect } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Spin, Alert } from 'antd'
import { BlogThemeFiles } from '../../services/api/blog'
import { renderBlogPage, RenderResult } from '../../utils/liquidRenderer'
import { getMockDataForView } from '../../utils/mockBlogData'
import { Workspace } from '../../services/api/types'

interface ThemePreviewProps {
  files: BlogThemeFiles
  workspace?: Workspace | null
  view: ViewType
}

type ViewType = 'home' | 'category' | 'post'

export function ThemePreview({ files, workspace, view }: ThemePreviewProps) {
  const { t } = useLingui()
  const [renderResult, setRenderResult] = useState<RenderResult | null>(null)
  const [isRendering, setIsRendering] = useState(false)

  useEffect(() => {
    const renderPreview = async () => {
      setIsRendering(true)
      try {
        const mockData = getMockDataForView(view)

        // Override mock data with actual workspace blog settings if available
        if (workspace?.settings?.blog_settings) {
          const blogSettings = workspace.settings.blog_settings
          if (blogSettings.title) {
            mockData.workspace.blog_title = blogSettings.title
          }
          if (blogSettings.logo_url) {
            mockData.workspace.logo_url = blogSettings.logo_url
          }
          if (blogSettings.icon_url) {
            mockData.workspace.icon_url = blogSettings.icon_url
          }
          if (blogSettings.seo) {
            mockData.seo = { ...mockData.seo, ...blogSettings.seo }
          }
        }

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
  }, [files, view, workspace, t])

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
          <Spin size="large" tip={t`Rendering preview...`} />
        </div>
      )}

      {!isRendering && renderResult && !renderResult.success && (
        <div style={{ padding: 24 }}>
          <Alert
            message={t`Template Error`}
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
          srcDoc={renderResult.html}
          style={{
            width: '100%',
            height: '100%',
            border: 'none',
            background: '#ffffff'
          }}
          title="Blog Preview"
          sandbox="allow-same-origin allow-scripts"
        />
      )}
    </div>
  )
}
