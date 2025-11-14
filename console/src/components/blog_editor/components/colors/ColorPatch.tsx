import type { ColorOption } from './ColorConstants'

export interface ColorPatchProps {
  color: ColorOption
  type: 'text' | 'background'
  onClick: () => void
}

/**
 * ColorPatch - Reusable color swatch button
 * Used in both toolbar and block actions menu
 */
export function ColorPatch({ color, type, onClick }: ColorPatchProps) {
  return (
    <button
      onClick={onClick}
      title={color.label}
      style={{
        width: '24px',
        height: '24px',
        borderRadius: '4px',
        backgroundColor: type === 'background' ? color.value || 'transparent' : 'transparent',
        border: type === 'text' ? `1px solid ${color.value || '#d9d9d9'}` : 'none',
        boxShadow: type === 'background' ? 'inset 0 0 0 1px rgba(0, 0, 0, 0.15)' : 'none',
        cursor: 'pointer',
        padding: 0,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: '10px',
        fontWeight: 'bold',
        color: type === 'text' ? color.value || '#000' : 'transparent',
        transition: 'transform 0.1s'
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.transform = 'scale(1.1)'
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.transform = 'scale(1)'
      }}
    >
      {type === 'text' && 'A'}
    </button>
  )
}
