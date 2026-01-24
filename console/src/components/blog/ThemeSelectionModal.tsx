import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Modal, Button } from 'antd'
import { FileOutlined, EyeOutlined } from '@ant-design/icons'
import { THEME_PRESETS, ThemePreset } from './themePresets'
import { ThemePresetPreviewDrawer } from './ThemePresetPreviewDrawer'
import { Workspace } from '../../services/api/types'

interface ThemeSelectionModalProps {
  open: boolean
  onClose: () => void
  onSelectTheme: (preset: ThemePreset) => void
  workspace?: Workspace | null
}

export function ThemeSelectionModal({
  open,
  onClose,
  onSelectTheme,
  workspace
}: ThemeSelectionModalProps) {
  const { t } = useLingui()
  const [previewPreset, setPreviewPreset] = useState<ThemePreset | null>(null)

  const handleSelectTheme = (preset: ThemePreset) => {
    onSelectTheme(preset)
    onClose()
  }

  const handlePreviewClick = (preset: ThemePreset, e: React.MouseEvent) => {
    e.stopPropagation()
    setPreviewPreset(preset)
  }

  const handleClosePreview = () => {
    setPreviewPreset(null)
  }

  return (
    <>
      <Modal
        title={t`Create New Theme`}
        open={open}
        onCancel={onClose}
        footer={null}
        width={1000}
        styles={{ body: { paddingTop: '24px' } }}
      >
        <p style={{ marginBottom: 24, color: '#595959' }}>
          {t`Choose a starting point for your new theme. You can customize everything later.`}
        </p>

        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(4, 1fr)',
            gap: 16
          }}
        >
          {THEME_PRESETS.map((preset) => (
            <div
              key={preset.id}
              onClick={() => handleSelectTheme(preset)}
              className="bg-white rounded-lg overflow-hidden transition-all duration-200 cursor-pointer hover:shadow-lg p-4 flex flex-col"
            >
              {/* Screenshot Placeholder */}
              <div
                className="w-full rounded flex flex-col items-center justify-center mb-4 relative"
                style={{
                  aspectRatio: '16 / 9',
                  backgroundColor: preset.placeholderColor
                }}
              >
                {preset.id === 'blank' ? (
                  <FileOutlined className="text-5xl text-gray-400 mb-2" />
                ) : null}
                <span className="text-sm text-gray-500">{t`Preview Coming Soon`}</span>
              </div>

              {/* Theme Info */}
              <div className="flex flex-col flex-grow">
                <h3 className="text-base font-semibold mb-2 text-black">{preset.name}</h3>
                <p className="text-sm text-gray-600 mb-3 leading-relaxed flex-grow">
                  {preset.description}
                </p>

                {/* Action Buttons */}
                <Button block icon={<EyeOutlined />} onClick={(e) => handlePreviewClick(preset, e)}>
                  {t`Preview`}
                </Button>
              </div>
            </div>
          ))}
        </div>
      </Modal>

      <ThemePresetPreviewDrawer
        open={!!previewPreset}
        onClose={handleClosePreview}
        preset={previewPreset}
        workspace={workspace}
        onSelectTheme={handleSelectTheme}
      />
    </>
  )
}
