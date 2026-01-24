import { Menu, Divider } from 'antd'
import {
  TeamOutlined,
  TagsOutlined,
  SettingOutlined,
  ExclamationCircleOutlined,
  MailOutlined
} from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'

export type SettingsSection =
  | 'team'
  | 'integrations'
  | 'webhooks'
  | 'custom-fields'
  | 'smtp-relay'
  | 'general'
  | 'blog'
  | 'danger-zone'

interface SettingsSidebarProps {
  activeSection: SettingsSection
  onSectionChange: (section: SettingsSection) => void
  isOwner: boolean
}

export function SettingsSidebar({ activeSection, onSectionChange, isOwner }: SettingsSidebarProps) {
  const { t } = useLingui()

  const menuItems = [
    {
      key: 'team',
      icon: <TeamOutlined />,
      label: t`Team`
    },
    {
      key: 'integrations',
      icon: (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="lucide lucide-blocks-icon lucide-blocks"
        >
          <path d="M10 22V7a1 1 0 0 0-1-1H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-5a1 1 0 0 0-1-1H2" />
          <rect x="14" y="2" width="8" height="8" rx="1" />
        </svg>
      ),
      label: t`Integrations`
    },
    {
      key: 'webhooks',
      icon: (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="lucide lucide-webhook"
        >
          <path d="M18 16.98h-5.99c-1.1 0-1.95.94-2.48 1.9A4 4 0 0 1 2 17c.01-.7.2-1.4.57-2" />
          <path d="m6 17 3.13-5.78c.53-.97.1-2.18-.5-3.1a4 4 0 1 1 6.89-4.06" />
          <path d="m12 6 3.13 5.73C15.66 12.7 16.9 13 18 13a4 4 0 0 1 0 8" />
        </svg>
      ),
      label: t`Webhooks`
    },
    {
      key: 'blog',
      icon: (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="lucide lucide-pen-line-icon lucide-pen-line"
        >
          <path d="M13 21h8" />
          <path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z" />
        </svg>
      ),
      label: t`Blog`
    },
    {
      key: 'custom-fields',
      icon: <TagsOutlined />,
      label: t`Custom Fields`
    },
    {
      key: 'smtp-relay',
      icon: <MailOutlined />,
      label: t`SMTP Relay`
    },
    {
      key: 'general',
      icon: <SettingOutlined />,
      label: t`General`
    }
  ]

  // Add danger zone only for owners
  if (isOwner) {
    menuItems.push({
      key: 'danger-zone',
      icon: <ExclamationCircleOutlined />,
      label: t`Danger Zone`
    })
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div className="text-xl font-medium pt-6 pl-6">{t`Settings`}</div>
      <Divider className="!my-4" />
      <Menu
        mode="inline"
        selectedKeys={[activeSection]}
        items={menuItems}
        onClick={({ key }) => onSectionChange(key as SettingsSection)}
        style={{ borderRight: 0, backgroundColor: '#F9F9F9' }}
      />
    </div>
  )
}
