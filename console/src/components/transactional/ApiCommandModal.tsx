import React from 'react'
import { Modal, Button, Alert, Tabs, Descriptions } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCopy } from '@fortawesome/free-solid-svg-icons'
import { Highlight, themes } from 'prism-react-renderer'
import { TransactionalNotification } from '../../services/api/transactional_notifications'
import { message } from 'antd'
import {
  curlSnippet,
  goSnippet,
  javaSnippet,
  pythonSnippet,
  smtpPayloadSnippet,
  typeScriptSnippet,
  type SnippetNotification
} from '../../lib/transactionalApiSnippets'
import { useLingui } from '@lingui/react/macro'

interface ApiCommandModalProps {
  open: boolean
  onClose: () => void
  notification: TransactionalNotification | null
  workspaceId: string
}

export const ApiCommandModal: React.FC<ApiCommandModalProps> = ({
  open,
  onClose,
  notification,
  workspaceId
}) => {
  const { t } = useLingui()
  const code = (build: (notification: SnippetNotification, workspaceId: string) => string) =>
    notification ? build(notification, workspaceId) : ''





  const renderSMTPInstructions = () => {
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
      <div className="space-y-6">
        <div>
          <h3 className="text-base font-semibold mb-3">{t`Connection Details`}</h3>
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label={t`Host`}>{smtpHost}</Descriptions.Item>
            <Descriptions.Item label={t`Port`}>{smtpPort}</Descriptions.Item>
            <Descriptions.Item label={t`Security`}>{securityLabel}</Descriptions.Item>
            <Descriptions.Item label={t`Username`}>
              {t`Your workspace API email (the email associated with your API key)`}
            </Descriptions.Item>
            <Descriptions.Item label={t`Password`}>{t`Your workspace API key`}</Descriptions.Item>
          </Descriptions>
        </div>

        <div>
          <h3 className="text-base font-semibold mb-3">{t`Email Body Payload`}</h3>
          <p className="mb-3 text-sm">
            {t`The email body must contain a JSON payload with your notification data. The SMTP envelope To/From addresses are ignored - the actual recipient is determined by`}{' '}
            <code>contact.email</code> {t`in the payload.`}
          </p>
          <CodeBlock code={code(smtpPayloadSnippet)} language="json" />
        </div>

        <div>
          <h3 className="text-base font-semibold mb-3">{t`Important Notes`}</h3>
          <ul className="list-disc list-inside space-y-1 text-sm">
            <li>
              <strong>{t`JSON Payload Required:`}</strong> {t`The email body must contain valid JSON matching the format above`}
            </li>
            <li>
              <strong>{t`Contact Email:`}</strong> {t`The`} <code>contact.email</code> {t`field is required`}
            </li>
            <li>
              <strong>{t`Deduplication:`}</strong> {t`Use`} <code>external_id</code> {t`to prevent duplicate sends`}
            </li>
            <li>
              <strong>{t`Template Variables:`}</strong> {t`Use`} <code>data</code> {t`for template variables`}
            </li>
            <li>
              <strong>{t`Email Options:`}</strong> {t`Supports reply_to, cc, bcc, and attachments`}
            </li>
          </ul>
        </div>
      </div>
    )
  }


  const handleCopyCommand = (code: string, language: string) => {
    navigator.clipboard
      .writeText(code)
      .then(() => {
        message.success(t`${language} code copied to clipboard!`)
      })
      .catch(() => {
        message.error(t`Failed to copy to clipboard`)
      })
  }

  const CodeBlock: React.FC<{ code: string; language: string }> = ({ code, language }) => (
    <Highlight theme={themes.github} code={code} language={language}>
      {({ className, style, tokens, getLineProps, getTokenProps }) => (
        <pre
          className={className}
          style={{
            ...style,
            fontSize: '12px',
            margin: 0,
            padding: '10px',
            maxHeight: '500px',
            overflow: 'auto'
          }}
        >
          {tokens.map((line, i) => (
            <div key={i} {...getLineProps({ line })}>
              <span
                style={{
                  display: 'inline-block',
                  width: '2em',
                  userSelect: 'none',
                  opacity: 0.3
                }}
              >
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

  const tabItems = [
    {
      key: 'curl',
      label: 'cURL',
      children: (
        <div>
          <p className="mb-4">
            {t`Use this curl command to send a transactional notification via API:`}
          </p>
          <CodeBlock code={code(curlSnippet)} language="bash" />
        </div>
      )
    },
    {
      key: 'typescript',
      label: 'TypeScript',
      children: (
        <div>
          <p className="mb-4">{t`Send a transactional notification using TypeScript/JavaScript:`}</p>
          <CodeBlock code={code(typeScriptSnippet)} language="typescript" />
        </div>
      )
    },
    {
      key: 'python',
      label: 'Python',
      children: (
        <div>
          <p className="mb-4">{t`Send a transactional notification using Python:`}</p>
          <CodeBlock code={code(pythonSnippet)} language="python" />
        </div>
      )
    },
    {
      key: 'golang',
      label: 'Go',
      children: (
        <div>
          <p className="mb-4">{t`Send a transactional notification using Go:`}</p>
          <CodeBlock code={code(goSnippet)} language="go" />
        </div>
      )
    },
    {
      key: 'java',
      label: 'Java',
      children: (
        <div>
          <p className="mb-4">{t`Send a transactional notification using Java:`}</p>
          <CodeBlock code={code(javaSnippet)} language="java" />
        </div>
      )
    },
    ...(window.SMTP_BRIDGE_ENABLED
      ? [
          {
            key: 'smtp',
            label: 'SMTP',
            children: (
              <div>
                <div className="mb-4">
                  <p className="text-sm">
                    {t`Send transactional notifications using SMTP bridge. Perfect for integrating with existing email systems or applications that support SMTP.`}
                  </p>
                </div>
                {renderSMTPInstructions()}
              </div>
            )
          }
        ]
      : [])
  ]

  const [activeTab, setActiveTab] = React.useState('curl')

  const getCurrentCode = () => {
    switch (activeTab) {
      case 'curl':
        return code(curlSnippet)
      case 'typescript':
        return code(typeScriptSnippet)
      case 'python':
        return code(pythonSnippet)
      case 'golang':
        return code(goSnippet)
      case 'java':
        return code(javaSnippet)
      case 'smtp':
        return code(smtpPayloadSnippet)
      default:
        return code(curlSnippet)
    }
  }

  const getCurrentLanguage = () => {
    switch (activeTab) {
      case 'curl':
        return 'cURL'
      case 'typescript':
        return 'TypeScript'
      case 'python':
        return 'Python'
      case 'golang':
        return 'Go'
      case 'java':
        return 'Java'
      case 'smtp':
        return 'JSON Payload'
      default:
        return 'cURL'
    }
  }

  return (
    <Modal
      title={t`API Command`}
      open={open}
      onCancel={onClose}
      footer={[
        <Button
          key="copy"
          type="primary"
          ghost
          icon={<FontAwesomeIcon icon={faCopy} />}
          onClick={() => handleCopyCommand(getCurrentCode(), getCurrentLanguage())}
        >
          {t`Copy ${getCurrentLanguage()} Code`}
        </Button>,
        <Button key="close" onClick={onClose}>
          {t`Close`}
        </Button>
      ]}
      width={900}
    >
      {notification && (
        <div>
          <Alert
            type="info"
            title={
              <div>
                <div>
                  {t`If the contact email doesn't exist in your workspace, it will be automatically created.`}
                </div>
                <div>
                  {t`Use`} <code>external_id</code> {t`for deduplication - notifications with the same external_id won't be sent twice.`}
                </div>
                <div>{t`All contact fields are optional except email.`}</div>
                <div>
                  {t`Use`} <code>data</code> {t`for template variables,`} <code>metadata</code> {t`for tracking (not available in templates).`}
                </div>
              </div>
            }
            className="!mb-4"
          />

          <Tabs activeKey={activeTab} onChange={setActiveTab} items={tabItems} size="small" />
        </div>
      )}
    </Modal>
  )
}
