import { useState } from 'react'
import { Drawer, Tabs, Button, Space } from 'antd'
import { ThemePreset } from './themePresets'
import { ThemePreview } from './ThemePreview'
import { Workspace } from '../../services/api/types'

interface ThemePresetPreviewDrawerProps {
  open: boolean
  onClose: () => void
  preset: ThemePreset | null
  workspace?: Workspace | null
  onSelectTheme: (preset: ThemePreset) => void
}

export function ThemePresetPreviewDrawer({
  open,
  onClose,
  preset,
  workspace,
  onSelectTheme
}: ThemePresetPreviewDrawerProps) {
  const [activeTab, setActiveTab] = useState<'home' | 'category' | 'post'>('post')

  if (!preset) return null

  const handleUseTheme = () => {
    onSelectTheme(preset)
    onClose()
  }

  return (
    <Drawer
      title={
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span>Preview: {preset.name}</span>
          <Space>
            <Button type="text" onClick={onClose}>
              Close
            </Button>
            <Button type="primary" onClick={handleUseTheme}>
              Use This Theme
            </Button>
          </Space>
        </div>
      }
      open={open}
      onClose={onClose}
      width={1000}
      styles={{ body: { padding: 0, height: 'calc(100vh - 55px)' } }}
    >
      <Tabs
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key as 'home' | 'category' | 'post')}
        type="card"
        style={{ height: '100%', display: 'flex', flexDirection: 'column' }}
        tabBarStyle={{ margin: 0, paddingLeft: 16, paddingRight: 16 }}
        items={[
          {
            key: 'home',
            label: 'Home',
            children: (
              <div style={{ height: 'calc(100vh - 110px)', overflow: 'auto' }}>
                <ThemePreview
                  files={preset.files}
                  styling={preset.styling}
                  workspace={workspace}
                  view="home"
                />
              </div>
            )
          },
          {
            key: 'category',
            label: 'Category',
            children: (
              <div style={{ height: 'calc(100vh - 110px)', overflow: 'auto' }}>
                <ThemePreview
                  files={preset.files}
                  styling={preset.styling}
                  workspace={workspace}
                  view="category"
                />
              </div>
            )
          },
          {
            key: 'post',
            label: 'Post',
            children: (
              <div style={{ height: 'calc(100vh - 110px)', overflow: 'auto' }}>
                <ThemePreview
                  files={preset.files}
                  styling={preset.styling}
                  workspace={workspace}
                  view="post"
                />
              </div>
            )
          }
        ]}
      />
    </Drawer>
  )
}
