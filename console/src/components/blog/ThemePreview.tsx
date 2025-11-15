import { useState, useEffect } from 'react'
import { Segmented, Spin, Alert } from 'antd'
import { BlogThemeFiles } from '../../services/api/blog'
import { renderBlogPage, RenderResult } from '../../utils/liquidRenderer'
import { getMockDataForView } from '../../utils/mockBlogData'
import { Workspace } from '../../services/api/types'

interface ThemePreviewProps {
  files: BlogThemeFiles
  styling?: any
  workspace?: Workspace | null
}

type ViewType = 'home' | 'category' | 'post'

export function ThemePreview({ files, styling, workspace }: ThemePreviewProps) {
  const [selectedView, setSelectedView] = useState<ViewType>('home')
  const [renderResult, setRenderResult] = useState<RenderResult | null>(null)
  const [isRendering, setIsRendering] = useState(false)

  useEffect(() => {
    const renderPreview = async () => {
      setIsRendering(true)
      try {
        const mockData = getMockDataForView(selectedView)
        
        // Override mock data with actual workspace blog settings if available
        if (workspace?.settings?.blog_settings) {
          const blogSettings = workspace.settings.blog_settings
          if (blogSettings.title) {
            mockData.blog.title = blogSettings.title
          }
          if (blogSettings.seo) {
            mockData.seo = { ...mockData.seo, ...blogSettings.seo }
          }
        }
        
        // Use styling from prop (theme styling) if provided
        if (styling) {
          mockData.styling = styling
        }
        
        const result = await renderBlogPage(files, selectedView, mockData)
        setRenderResult(result)
      } catch (error) {
        console.error('Preview rendering failed:', error)
        setRenderResult({
          success: false,
          error: 'Failed to render preview'
        })
      } finally {
        setIsRendering(false)
      }
    }

    renderPreview()
  }, [files, selectedView, styling, workspace])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* View Selector */}
      <div
        style={{
          padding: '12px 16px',
          borderBottom: '1px solid #f0f0f0',
          background: '#fafafa'
        }}
      >
        <Segmented
          value={selectedView}
          onChange={(value) => setSelectedView(value as ViewType)}
          options={[
            { label: 'Home', value: 'home' },
            { label: 'Category', value: 'category' },
            { label: 'Post', value: 'post' }
          ]}
          block
        />
      </div>

      {/* Preview Content */}
      <div
        style={{
          flex: 1,
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
            <Spin size="large" tip="Rendering preview..." />
          </div>
        )}

        {!isRendering && renderResult && !renderResult.success && (
          <div style={{ padding: 24 }}>
            <Alert
              message="Template Error"
              description={
                <div>
                  <p>{renderResult.error}</p>
                  {renderResult.errorLine && <p>Line: {renderResult.errorLine}</p>}
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
            sandbox="allow-same-origin"
          />
        )}
      </div>
    </div>
  )
}

