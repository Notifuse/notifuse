import { Divider } from 'antd'

interface SettingsSectionHeaderProps {
  title: string
  description: string
  /** Extra classes for the trailing divider, e.g. `!mb-4` to tighten the gap. */
  className?: string
}

export function SettingsSectionHeader({
  title,
  description,
  className
}: SettingsSectionHeaderProps) {
  return (
    <>
      <div className="text-2xl font-medium mb-2">{title}</div>
      <div className="text-gray-500">{description}</div>

      <Divider className={className ? `mb-12 ${className}` : 'mb-12'} />
    </>
  )
}
