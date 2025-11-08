import { Divider } from 'antd'

interface SettingsSectionHeaderProps {
  title: string
  description: string
}

export function SettingsSectionHeader({ title, description }: SettingsSectionHeaderProps) {
  return (
    <>
      <div className="text-2xl font-medium mb-2">{title}</div>
      <div className="text-gray-500">{description}</div>

      <Divider className="mb-12" />
    </>
  )
}
