import { useState, type ReactNode } from 'react'
import { App, Button } from 'antd'
import { Copy } from 'lucide-react'
import { Highlight, themes } from 'prism-react-renderer'
import { useLingui } from '@lingui/react/macro'
import { KeyValueList, Mono, Panel, SidePanel } from '../layout'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'
import { copyToClipboard } from '../../lib/copyToClipboard'
import {
  curlSnippet,
  goSnippet,
  javaSnippet,
  pythonSnippet,
  smtpPayloadSnippet,
  typeScriptSnippet,
  type SnippetNotification
} from '../../lib/transactionalApiSnippets'

type SnippetKey = 'curl' | 'typescript' | 'python' | 'golang' | 'java' | 'smtp'

interface Snippet {
  key: SnippetKey
  /** The tab's label, a language name left untranslated. */
  label: string
  /** What the copy button and its message name. */
  copyLabel: string
  prism: string
  build: (notification: SnippetNotification, workspaceId: string) => string
}

const SNIPPETS: Snippet[] = [
  { key: 'curl', label: 'cURL', copyLabel: 'cURL', prism: 'bash', build: curlSnippet },
  { key: 'typescript', label: 'TypeScript', copyLabel: 'TypeScript', prism: 'typescript', build: typeScriptSnippet },
  { key: 'python', label: 'Python', copyLabel: 'Python', prism: 'python', build: pythonSnippet },
  { key: 'golang', label: 'Go', copyLabel: 'Go', prism: 'go', build: goSnippet },
  { key: 'java', label: 'Java', copyLabel: 'Java', prism: 'java', build: javaSnippet },
  { key: 'smtp', label: 'SMTP', copyLabel: 'JSON Payload', prism: 'json', build: smtpPayloadSnippet }
]

/** A highlighted sample with line numbers; it scrolls sideways, the panel's body scrolls down. */
function CodeBlock({ code, language }: { code: string; language: string }) {
  return (
    <Highlight theme={themes.github} code={code} language={language}>
      {({ className, style, tokens, getLineProps, getTokenProps }) => (
        <pre
          data-testid="api-command-code"
          className={`${className} m-0 overflow-x-auto rounded-sm p-2.5 text-xs`}
          style={style}
        >
          {tokens.map((line, i) => (
            <div key={i} {...getLineProps({ line })}>
              <span data-line-number aria-hidden="true" className="inline-block w-6 opacity-30 select-none">
                {i + 1}
              </span>
              {line.map((token, key) => (
                <span key={key} {...getTokenProps({ token })} />
              ))}
            </div>
          ))}
        </pre>
      )}
    </Highlight>
  )
}

function Notes({ title, children }: { title: ReactNode; children: ReactNode }) {
  return (
    <Panel title={title}>
      <ul className="m-0 flex list-disc flex-col gap-1 pl-4 text-sm text-ink-2">
        {children}
      </ul>
    </Panel>
  )
}

interface ApiCommandDrawerProps {
  open: boolean
  onClose: () => void
  /** A saved notification, or the one being created: its identifier, its name and the data to send. */
  notification: SnippetNotification & Pick<TransactionalNotification, 'name'>
  workspaceId: string
}

/** How to send a notification through the API, in every language, in a side panel. */
export function ApiCommandDrawer({ open, onClose, notification, workspaceId }: ApiCommandDrawerProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const smtpEnabled = Boolean(window.SMTP_BRIDGE_ENABLED)
  const snippets = SNIPPETS.filter((snippet) => snippet.key !== 'smtp' || smtpEnabled)
  const [activeKey, setActiveKey] = useState<SnippetKey>('curl')
  const current = snippets.find((snippet) => snippet.key === activeKey) ?? snippets[0]
  const code = (snippet: Snippet) => snippet.build(notification, workspaceId)

  const handleCopy = async () => {
    const language = current.copyLabel
    if (await copyToClipboard(code(current))) message.success(t`${language} code copied to clipboard!`)
    else message.error(t`Failed to copy to clipboard`)
  }

  const intros: Record<Exclude<SnippetKey, 'smtp'>, string> = {
    curl: t`Use this curl command to send a transactional notification via API:`,
    typescript: t`Send a transactional notification using TypeScript/JavaScript:`,
    python: t`Send a transactional notification using Python:`,
    golang: t`Send a transactional notification using Go:`,
    java: t`Send a transactional notification using Java:`
  }

  // What every way of sending shares: the contact, deduplication, data and metadata.
  const generalNotes = (
    <Notes title={t`Good to know`}>
      <li>{t`If the contact email doesn't exist in your workspace, it will be automatically created.`}</li>
      <li>
        {t`Use`} <Mono>external_id</Mono>{' '}
        {t`for deduplication - notifications with the same external_id won't be sent twice.`}
      </li>
      <li>{t`All contact fields are optional except email.`}</li>
      <li>
        {t`Use`} <Mono>data</Mono> {t`for template variables,`} <Mono>metadata</Mono>{' '}
        {t`for tracking (not available in templates).`}
      </li>
    </Notes>
  )

  const smtpBody = () => {
    const smtpHost = window.SMTP_BRIDGE_DOMAIN || 'your-smtp-domain.com'
    const smtpPort = window.SMTP_BRIDGE_PORT || 587
    const tlsMode = window.SMTP_BRIDGE_TLS_MODE
    const securityLabel =
      tlsMode === 'implicit'
        ? t`Implicit TLS (SMTPS)`
        : tlsMode === 'off'
          ? t`Plain text (only safe behind a TLS-terminating proxy)`
          : t`STARTTLS required`
    return (
      <>
        <p data-unreset className="m-0 text-sm text-ink-3">
          {t`Send transactional notifications using SMTP bridge. Perfect for integrating with existing email systems or applications that support SMTP.`}
        </p>
        <Panel title={t`Connection Details`}>
          <KeyValueList
            labelWidth={96}
            items={[
              { label: t`Host`, value: <Mono copy>{smtpHost}</Mono> },
              { label: t`Port`, value: <Mono copy>{smtpPort}</Mono> },
              { label: t`Security`, value: securityLabel },
              { label: t`Username`, value: t`Your workspace API email (the email associated with your API key)` },
              { label: t`Password`, value: t`Your workspace API key` }
            ]}
          />
        </Panel>
        <Panel title={t`Email Body Payload`}>
          <div className="flex flex-col gap-2">
            <p data-unreset className="m-0 text-sm text-ink-3">
              {t`The email body must contain a JSON payload with your notification data. The SMTP envelope To/From addresses are ignored - the actual recipient is determined by`}{' '}
              <Mono>contact.email</Mono> {t`in the payload.`}
            </p>
            <CodeBlock code={smtpPayloadSnippet(notification, workspaceId)} language="json" />
          </div>
        </Panel>
        <Notes title={t`Important Notes`}>
          <li>
            <strong>{t`JSON Payload Required:`}</strong>{' '}
            {t`The email body must contain valid JSON matching the format above`}
          </li>
          <li>
            <strong>{t`Contact Email:`}</strong> {t`The`} <Mono>contact.email</Mono> {t`field is required`}
          </li>
          <li>
            <strong>{t`Deduplication:`}</strong> {t`Use`} <Mono>external_id</Mono> {t`to prevent duplicate sends`}
          </li>
          <li>
            <strong>{t`Template Variables:`}</strong> {t`Use`} <Mono>data</Mono> {t`for template variables`}
          </li>
          <li>
            <strong>{t`Email Options:`}</strong> {t`Supports reply_to, cc, bcc, and attachments`}
          </li>
        </Notes>
      </>
    )
  }

  const body = (snippet: Snippet) => (
    <div className="flex flex-col gap-3">
      {snippet.key === 'smtp' ? (
        smtpBody()
      ) : (
        <>
          <p data-unreset className="m-0 text-sm text-ink-3">{intros[snippet.key]}</p>
          <Panel flush>
            <CodeBlock code={code(snippet)} language={snippet.prism} />
          </Panel>
        </>
      )}
      {generalNotes}
    </div>
  )

  return (
    <SidePanel
      open={open}
      onClose={onClose}
      title={t`API command`}
      meta={[notification.name, <Mono key="id">{notification.id}</Mono>]}
      copyLink={false}
      tabs={{
        activeKey: current.key,
        onChange: (key) => setActiveKey(key as SnippetKey),
        items: snippets.map((snippet) => ({ key: snippet.key, label: snippet.label, children: body(snippet) }))
      }}
      footer={
        <>
          <Button onClick={onClose}>{t`Close`}</Button>
          <Button type="primary" icon={<Copy size={14} aria-hidden="true" />} onClick={() => void handleCopy()}>
            {t`Copy ${current.copyLabel} Code`}
          </Button>
        </>
      }
    />
  )
}
