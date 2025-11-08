import { Descriptions } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'

export function SMTPRelaySettings() {
  return (
    <>
      <div className="text-2xl font-medium mb-2">SMTP Relay</div>
      <div className="text-gray-500 mb-16">
        SMTP relay server for forwarding transactional emails
      </div>

      {window.SMTP_RELAY_ENABLED ? (
        <>
          <div style={{ marginBottom: '16px' }}>
            <a
              href="https://docs.notifuse.com/concepts/transactional-api#smtp-relay"
              target="_blank"
              rel="noopener noreferrer"
            >
              View SMTP Relay documentation and setup guide
            </a>
          </div>
          <Descriptions
            bordered
            column={1}
            size="small"
            labelStyle={{ width: '200px', fontWeight: '500' }}
          >
            <Descriptions.Item label="SMTP domain">
              {window.SMTP_RELAY_DOMAIN || 'Not set'}
            </Descriptions.Item>

            <Descriptions.Item label="SMTP port">
              {window.SMTP_RELAY_PORT || 'Not set'}
            </Descriptions.Item>

            <Descriptions.Item label="TLS">
              {window.SMTP_RELAY_TLS_ENABLED ? (
                <span style={{ color: '#52c41a' }}>
                  <CheckCircleOutlined style={{ marginRight: '8px' }} />
                  Enabled
                </span>
              ) : (
                <span style={{ color: '#ff4d4f' }}>
                  <CloseCircleOutlined style={{ marginRight: '8px' }} />
                  Disabled
                </span>
              )}
            </Descriptions.Item>
          </Descriptions>
        </>
      ) : (
        <div style={{ color: '#8c8c8c', fontStyle: 'italic' }}>
          SMTP relay is not configured.{' '}
          <a
            href="https://docs.notifuse.com/installation#smtp-relay-configuration"
            target="_blank"
            rel="noopener noreferrer"
          >
            Learn how to enable SMTP relay
          </a>
        </div>
      )}
    </>
  )
}
