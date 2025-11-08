import { Menu, Typography } from 'antd'
import {
  TeamOutlined,
  ApiOutlined,
  TagsOutlined,
  SettingOutlined,
  GlobalOutlined,
  ExclamationCircleOutlined,
  MailOutlined
} from '@ant-design/icons'

const { Title } = Typography

export type SettingsSection =
  | 'team'
  | 'integrations'
  | 'custom-fields'
  | 'smtp-relay'
  | 'general'
  | 'web-publications'
  | 'danger-zone'

interface SettingsSidebarProps {
  activeSection: SettingsSection
  onSectionChange: (section: SettingsSection) => void
  isOwner: boolean
}

export function SettingsSidebar({ activeSection, onSectionChange, isOwner }: SettingsSidebarProps) {
  const menuItems = [
    {
      key: 'team',
      icon: <TeamOutlined />,
      label: 'Team'
    },
    {
      key: 'integrations',
      icon: <ApiOutlined />,
      label: 'Integrations'
    },
    {
      key: 'custom-fields',
      icon: <TagsOutlined />,
      label: 'Custom Fields'
    },
    {
      key: 'smtp-relay',
      icon: <MailOutlined />,
      label: 'SMTP Relay'
    },
    {
      key: 'general',
      icon: <SettingOutlined />,
      label: 'General'
    },
    {
      key: 'web-publications',
      icon: <GlobalOutlined />,
      label: 'Web Publications'
    }
  ]

  // Add danger zone only for owners
  if (isOwner) {
    menuItems.push({
      key: 'danger-zone',
      icon: <ExclamationCircleOutlined />,
      label: 'Danger Zone'
    })
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div className="text-xl font-medium p-6 pb-4">Settings</div>

      <Menu
        mode="inline"
        selectedKeys={[activeSection]}
        items={menuItems}
        onClick={({ key }) => onSectionChange(key as SettingsSection)}
        style={{ borderRight: 0 }}
      />
    </div>
  )
}
