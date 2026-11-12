import { useState } from 'react'
import { App, Button, Segmented } from 'antd'
import { Highlight, themes } from 'prism-react-renderer'
import { useLingui } from '@lingui/react/macro'
import { Panel } from '../layout'
import { copyToClipboard } from '../../lib/copyToClipboard'
import {
  curlSnippet,
  pythonSnippet,
  typeScriptSnippet,
  type SnippetNotification
} from '../../lib/transactionalApiSnippets'

type Language = 'curl' | 'typescript' | 'python'

const SNIPPETS: Record<Language, { label: string; prism: string; build: typeof curlSnippet }> = {
  curl: { label: 'cURL', prism: 'bash', build: curlSnippet },
  typescript: { label: 'TypeScript', prism: 'typescript', build: typeScriptSnippet },
  python: { label: 'Python', prism: 'python', build: pythonSnippet }
}

interface ApiCallPanelProps {
  notification: SnippetNotification
  workspaceId: string
  /** Opens the API command modal with every language. */
  onOpenAll: () => void
}

/** The call that sends the notification, in three languages. */
export function ApiCallPanel({ notification, workspaceId, onOpenAll }: ApiCallPanelProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [language, setLanguage] = useState<Language>('curl')
  const snippet = SNIPPETS[language]
  const code = snippet.build(notification, workspaceId)

  const copy = async () => {
    if (await copyToClipboard(code)) message.success(t`Code copied`)
    else message.error(t`Failed to copy to clipboard`)
  }

  return (
    <Panel
      title={t`API call`}
      extra={
        <Segmented<Language>
          size="small"
          value={language}
          onChange={setLanguage}
          options={(Object.keys(SNIPPETS) as Language[]).map((key) => ({ label: SNIPPETS[key].label, value: key }))}
        />
      }
    >
      <div className="flex flex-col gap-2">
        <Highlight theme={themes.github} code={code} language={snippet.prism}>
          {({ className, style, tokens, getLineProps, getTokenProps }) => (
            <pre
              data-testid="api-call-code"
              className={`${className} m-0 max-h-[280px] overflow-auto rounded-sm p-2.5 text-xs`}
              style={style}
            >
              {tokens.map((line, i) => (
                <div key={i} {...getLineProps({ line })}>
                  {line.map((token, key) => (
                    <span key={key} {...getTokenProps({ token })} />
                  ))}
                </div>
              ))}
            </pre>
          )}
        </Highlight>
        <div className="flex items-center gap-2">
          <Button size="small" onClick={() => void copy()}>
            {t`Copy`}
          </Button>
          <Button size="small" type="link" onClick={onOpenAll}>
            {t`All languages`}
          </Button>
        </div>
      </div>
    </Panel>
  )
}
