import { Descriptions } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { SettingsSectionHeader } from './SettingsSectionHeader'

export function SMTPRelaySettings() {
  const { t } = useLingui()

  return (
    <>
      <SettingsSectionHeader
        title={t`SMTP Relay`}
        description={t`SMTP relay server for forwarding transactional emails`}
      />

      {window.SMTP_RELAY_ENABLED ? (
        <>
          <div style={{ marginBottom: '16px' }}>
            <a
              href="https://docs.notifuse.com/concepts/transactional-api#smtp-relay"
              target="_blank"
              rel="noopener noreferrer"
            >
              {t`View SMTP Relay documentation and setup guide`}
            </a>
          </div>
          <Descriptions
            bordered
            column={1}
            size="small"
            styles={{ label: { width: '200px', fontWeight: '500' } }}
          >
            <Descriptions.Item label={t`SMTP domain`}>
              {window.SMTP_RELAY_DOMAIN || t`Not set`}
            </Descriptions.Item>

            <Descriptions.Item label={t`SMTP port`}>
              {window.SMTP_RELAY_PORT || t`Not set`}
            </Descriptions.Item>

            <Descriptions.Item label={t`TLS`}>
              {window.SMTP_RELAY_TLS_ENABLED ? (
                <span style={{ color: '#52c41a' }}>
                  <CheckCircleOutlined style={{ marginRight: '8px' }} />
                  {t`Enabled`}
                </span>
              ) : (
                <span style={{ color: '#ff4d4f' }}>
                  <CloseCircleOutlined style={{ marginRight: '8px' }} />
                  {t`Disabled`}
                </span>
              )}
            </Descriptions.Item>
          </Descriptions>
        </>
      ) : (
        <div style={{ color: '#8c8c8c', fontStyle: 'italic' }}>
          {t`SMTP relay is not configured.`}{' '}
          <a
            href="https://docs.notifuse.com/installation#smtp-relay-configuration"
            target="_blank"
            rel="noopener noreferrer"
          >
            {t`Learn how to enable SMTP relay`}
          </a>
        </div>
      )}
    </>
  )
}
