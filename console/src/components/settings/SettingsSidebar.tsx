import type { ReactNode } from 'react'
import type { MessageDescriptor } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import {
  BadgeCheck,
  Blocks,
  BookOpen,
  Braces,
  ChartLine,
  Mail,
  ScrollText,
  ShieldAlert,
  SlidersHorizontal,
  Users,
  Webhook
} from 'lucide-react'
import { RailItem, RailSection } from '../layout'

/**
 * The rail's groups, in their order. The menu keys are cast to SettingsSection on click, so a
 * stale key here would reach the router unchallenged: the sections stay a runtime value a test
 * walks, and the type derives from it.
 */
export const SETTINGS_GROUPS = [
  { title: msg`Workspace`, sections: ['general', 'team'] },
  { title: msg`Sending`, sections: ['integrations', 'webhooks', 'smtp-bridge'] },
  { title: msg`Content`, sections: ['custom-fields', 'blog', 'web-analytics'] },
  // Licence is deployment-wide rather than workspace-wide, and rendered only for a root user —
  // but listed here unconditionally, because this list is what makes a URL valid and a root user
  // must be able to reach /settings/licence from the banner whatever workspace they are in.
  { title: msg`Security`, sections: ['audit-logs', 'licence', 'danger-zone'] }
] as const satisfies ReadonlyArray<{ title: MessageDescriptor; sections: readonly string[] }>

export type SettingsSection = (typeof SETTINGS_GROUPS)[number]['sections'][number]

export const SETTINGS_SECTIONS: readonly SettingsSection[] = SETTINGS_GROUPS.flatMap(
  (group) => group.sections
)

/** The section `/settings` and an unknown section open. */
export const DEFAULT_SETTINGS_SECTION: SettingsSection = 'general'

const ICON_SIZE = 15

const ICONS: Record<SettingsSection, ReactNode> = {
  general: <SlidersHorizontal size={ICON_SIZE} />,
  team: <Users size={ICON_SIZE} />,
  integrations: <Blocks size={ICON_SIZE} />,
  webhooks: <Webhook size={ICON_SIZE} />,
  'smtp-bridge': <Mail size={ICON_SIZE} />,
  'custom-fields': <Braces size={ICON_SIZE} />,
  blog: <BookOpen size={ICON_SIZE} />,
  'web-analytics': <ChartLine size={ICON_SIZE} />,
  'audit-logs': <ScrollText size={ICON_SIZE} />,
  licence: <BadgeCheck size={ICON_SIZE} />,
  'danger-zone': <ShieldAlert size={ICON_SIZE} />
}

const LABELS: Record<SettingsSection, MessageDescriptor> = {
  general: msg`General`,
  team: msg`Team & API keys`,
  integrations: msg`Integrations`,
  webhooks: msg`Webhooks`,
  'smtp-bridge': msg`SMTP bridge`,
  'custom-fields': msg`Custom fields`,
  blog: msg`Blog`,
  'web-analytics': msg`Web analytics`,
  'audit-logs': msg`Audit log`,
  licence: msg`Licence`,
  'danger-zone': msg`Danger zone`
}

interface SettingsSidebarProps {
  activeSection: SettingsSection
  onSectionChange: (section: SettingsSection) => void
  isOwner: boolean
  // Root, not owner: the licence covers the deployment, and the server's own licence endpoints
  // are gated on requireRootUser rather than on workspace ownership.
  isRoot: boolean
}

export function SettingsSidebar({ activeSection, onSectionChange, isOwner, isRoot }: SettingsSidebarProps) {
  const { i18n } = useLingui()

  const visible = (section: SettingsSection) => {
    // Retention and deletion are an owner's decisions; the log itself is read under Logs.
    if (section === 'audit-logs' || section === 'danger-zone') return isOwner
    if (section === 'licence') return isRoot
    return true
  }

  // The rail's title ("Settings") is drawn by RailLayout, which names the navigation after it.
  return (
    <div className="flex flex-col">
      {SETTINGS_GROUPS.map((group) => {
        const sections = group.sections.filter(visible)
        // A group without a visible entry disappears (Security for a member who is not root).
        if (sections.length === 0) return null
        return (
          <RailSection key={group.sections[0]} title={i18n._(group.title)}>
            {sections.map((section) => (
              <RailItem
                key={section}
                icon={ICONS[section]}
                label={i18n._(LABELS[section])}
                active={section === activeSection}
                tone={section === 'danger-zone' ? 'danger' : undefined}
                onClick={() => onSectionChange(section)}
              />
            ))}
          </RailSection>
        )
      })}
    </div>
  )
}
