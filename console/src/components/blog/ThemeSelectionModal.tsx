import { Modal, Card } from 'antd'
import { FileOutlined } from '@ant-design/icons'
import { THEME_PRESETS, ThemePreset } from './themePresets'

interface ThemeSelectionModalProps {
  open: boolean
  onClose: () => void
  onSelectTheme: (preset: ThemePreset) => void
}

export function ThemeSelectionModal({ open, onClose, onSelectTheme }: ThemeSelectionModalProps) {
  const handleSelectTheme = (preset: ThemePreset) => {
    onSelectTheme(preset)
    onClose()
  }

  return (
    <Modal
      title="Create New Theme"
      open={open}
      onCancel={onClose}
      footer={null}
      width={900}
      styles={{ body: { padding: '24px' } }}
    >
      <p style={{ marginBottom: 24, color: '#595959' }}>
        Choose a starting point for your new theme. You can customize everything later.
      </p>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(380px, 1fr))',
          gap: 16
        }}
      >
        {THEME_PRESETS.map((preset) => (
          <Card
            key={preset.id}
            hoverable
            onClick={() => handleSelectTheme(preset)}
            style={{
              borderRadius: 8,
              overflow: 'hidden',
              transition: 'all 0.2s ease',
              cursor: 'pointer'
            }}
            bodyStyle={{ padding: 16 }}
            onMouseEnter={(e) => {
              e.currentTarget.style.borderColor = '#1890ff'
              e.currentTarget.style.boxShadow = '0 4px 12px rgba(24, 144, 255, 0.15)'
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.borderColor = '#d9d9d9'
              e.currentTarget.style.boxShadow = 'none'
            }}
          >
            {/* Screenshot Placeholder */}
            <div
              style={{
                width: '100%',
                aspectRatio: '16 / 9',
                backgroundColor: preset.placeholderColor,
                borderRadius: 4,
                border: '1px solid #d9d9d9',
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                marginBottom: 16,
                position: 'relative'
              }}
            >
              {preset.id === 'blank' ? (
                <FileOutlined style={{ fontSize: 48, color: '#bfbfbf', marginBottom: 8 }} />
              ) : null}
              <span style={{ fontSize: 14, color: '#8c8c8c' }}>Preview Coming Soon</span>
            </div>

            {/* Theme Info */}
            <div>
              <h3 style={{ fontSize: 16, fontWeight: 600, marginBottom: 8, color: '#000' }}>
                {preset.name}
              </h3>
              <p style={{ fontSize: 14, color: '#595959', margin: 0, lineHeight: 1.5 }}>
                {preset.description}
              </p>
            </div>
          </Card>
        ))}
      </div>
    </Modal>
  )
}
