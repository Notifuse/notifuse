import { useLingui } from '@lingui/react/macro'
import { ColorPatch } from './ColorPatch'
import { TEXT_COLORS, BACKGROUND_COLORS } from './ColorConstants'
import type { RecentColor } from '../../toolbars/components/useRecentColors'

export interface ColorGridProps {
  onTextColorChange: (value: string | null, label: string) => void
  onBackgroundColorChange: (value: string | null, label: string) => void
  showTextColors?: boolean
  showBackgroundColors?: boolean
  recentColors?: RecentColor[]
  isInitialized?: boolean
}

/**
 * ColorGrid - Grid layout for color patches
 * Displays recently used, text, and background colors
 */
export function ColorGrid({
  onTextColorChange,
  onBackgroundColorChange,
  showTextColors = true,
  showBackgroundColors = true,
  recentColors = [],
  isInitialized = false
}: ColorGridProps) {
  const { t } = useLingui()
  // Split colors into two rows of 5
  const textColorsRow1 = TEXT_COLORS.slice(0, 5)
  const textColorsRow2 = TEXT_COLORS.slice(5, 10)
  const backgroundColorsRow1 = BACKGROUND_COLORS.slice(0, 5)
  const backgroundColorsRow2 = BACKGROUND_COLORS.slice(5, 10)

  return (
    <div style={{ width: '180px' }}>
      {/* Recently Used Section */}
      {isInitialized && recentColors.length > 0 && (
        <div style={{ marginBottom: '12px' }}>
          <div
            style={{
              fontSize: '12px',
              fontWeight: '600',
              color: '#8c8c8c',
              padding: '8px 12px 4px',
              textTransform: 'uppercase'
            }}
          >
            {t`Recently Used`}
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', padding: '0 12px' }}>
            {recentColors.slice(0, 5).map((recentColor, index) => (
              <button
                key={`${recentColor.type}-${recentColor.value}-${index}`}
                onClick={() => {
                  if (recentColor.type === 'text') {
                    onTextColorChange(recentColor.value, recentColor.label)
                  } else {
                    onBackgroundColorChange(recentColor.value, recentColor.label)
                  }
                }}
                title={`${recentColor.label} ${
                  recentColor.type === 'text' ? t`text` : t`background`
                }`}
                style={{
                  width: '24px',
                  height: '24px',
                  borderRadius: '4px',
                  backgroundColor:
                    recentColor.type === 'background' ? recentColor.value : 'transparent',
                  border: recentColor.type === 'text' ? `1px solid ${recentColor.value}` : 'none',
                  boxShadow:
                    recentColor.type === 'background'
                      ? 'inset 0 0 0 1px rgba(0, 0, 0, 0.15)'
                      : 'none',
                  cursor: 'pointer',
                  padding: 0,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: '10px',
                  fontWeight: 'bold',
                  color: recentColor.type === 'text' ? recentColor.value : 'transparent',
                  transition: 'transform 0.1s'
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.transform = 'scale(1.1)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.transform = 'scale(1)'
                }}
              >
                {recentColor.type === 'text' && 'A'}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Text Colors Section */}
      {showTextColors && (
        <div style={{ marginBottom: '12px' }}>
          <div
            style={{
              fontSize: '12px',
              fontWeight: '600',
              color: '#8c8c8c',
              padding: '8px 12px 4px',
              textTransform: 'uppercase'
            }}
          >
            {t`Text Color`}
          </div>
          <div style={{ padding: '0 12px' }}>
            <div style={{ display: 'flex', gap: '8px', marginBottom: '8px' }}>
              {textColorsRow1.map((color) => (
                <ColorPatch
                  key={color.value}
                  color={color}
                  type="text"
                  onClick={() => onTextColorChange(color.value, color.label)}
                />
              ))}
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              {textColorsRow2.map((color) => (
                <ColorPatch
                  key={color.value}
                  color={color}
                  type="text"
                  onClick={() => onTextColorChange(color.value, color.label)}
                />
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Background Colors Section */}
      {showBackgroundColors && (
        <div>
          <div
            style={{
              fontSize: '12px',
              fontWeight: '600',
              color: '#8c8c8c',
              padding: '8px 12px 4px',
              textTransform: 'uppercase'
            }}
          >
            {t`Background Color`}
          </div>
          <div style={{ padding: '0 12px' }}>
            <div style={{ display: 'flex', gap: '8px', marginBottom: '8px' }}>
              {backgroundColorsRow1.map((color) => (
                <ColorPatch
                  key={color.value}
                  color={color}
                  type="background"
                  onClick={() => onBackgroundColorChange(color.value, color.label)}
                />
              ))}
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              {backgroundColorsRow2.map((color) => (
                <ColorPatch
                  key={color.value}
                  color={color}
                  type="background"
                  onClick={() => onBackgroundColorChange(color.value, color.label)}
                />
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
