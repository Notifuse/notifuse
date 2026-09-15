import { useLingui } from '@lingui/react/macro'
import Editor from '@monaco-editor/react'

interface PlainTextEditorPanelProps {
  value: string
  onChange: (value: string) => void
  height?: string | number
}

/**
 * The plain-text (text/plain) alternative editor, shared between MjmlCodeEditor's
 * "Plain text" tab and EmailBuilder's "Plain text" view mode so both editing surfaces —
 * and every language's nested editor Drawer that reuses them — get the same experience.
 */
const PlainTextEditorPanel: React.FC<PlainTextEditorPanelProps> = ({
  value,
  onChange,
  height = '100%'
}) => {
  const { t } = useLingui()

  return (
    <div style={{ height, position: 'relative' }}>
      {!value && (
        <div
          style={{
            position: 'absolute',
            inset: 0,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: 40,
            pointerEvents: 'none',
            zIndex: 1
          }}
        >
          <p style={{ color: '#999', textAlign: 'center', maxWidth: 480, margin: 0 }}>
            {t`Sent as the text/plain part alongside the HTML body — improves deliverability and accessibility. Supports the same templating variables as the subject. Leave blank to send HTML only.`}
          </p>
        </div>
      )}
      <Editor
        height="100%"
        language="plaintext"
        theme="vs"
        value={value}
        onChange={(v) => onChange(v || '')}
        options={{
          minimap: { enabled: false },
          fontSize: 13,
          lineNumbers: 'on',
          wordWrap: 'on',
          scrollBeyondLastLine: false,
          automaticLayout: true,
          scrollbar: {
            vertical: 'visible',
            horizontal: 'visible'
          }
        }}
      />
    </div>
  )
}

export default PlainTextEditorPanel
