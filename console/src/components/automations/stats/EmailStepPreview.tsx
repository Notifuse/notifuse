import { Button, Skeleton } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { ExternalLink, Eye } from 'lucide-react'
import { KeyValueList } from '../../layout'
import { templatesApi } from '../../../services/api/template'
import type { EmailNodeConfig } from '../../../services/api/automation'
import type { Workspace } from '../../../services/api/types'
import { TemplatePreviewFrame } from '../../broadcasts/detail/TemplatePreviewFrame'
import { formatSender, resolveVariationSender } from '../../broadcasts/variationSender'
import { TemplatePreviewPopover } from '../../templates'

interface EmailStepPreviewProps {
  workspace: Workspace
  config: Partial<EmailNodeConfig>
  canReadTemplates: boolean
}

/** The sender, the subject and a render of the step's template, with links to preview and open it. */
export function EmailStepPreview({ workspace, config, canReadTemplates }: EmailStepPreviewProps) {
  const { t } = useLingui()
  const templateId = config.template_id
  const query = useQuery({
    queryKey: ['template', workspace.id, templateId],
    queryFn: () => templatesApi.get({ workspace_id: workspace.id, id: templateId! }),
    enabled: !!templateId && canReadTemplates
  })

  if (!templateId) return null
  if (!canReadTemplates) {
    return <p data-unreset className="m-0 text-sm text-ink-3">{t`You need access to templates to preview this email.`}</p>
  }
  if (query.isLoading) return <Skeleton active title={false} paragraph={{ rows: 3 }} />
  const template = query.data?.template
  if (!template) return <p data-unreset className="m-0 text-sm text-ink-3">{t`This template no longer exists`}</p>

  // The step's integration, else the workspace's marketing one, as the server sends.
  const integrationId = config.integration_id || workspace.settings?.marketing_email_provider_id
  const senders = workspace.integrations?.find((i) => i.id === integrationId)?.email_provider?.senders ?? []
  const sender = resolveVariationSender(senders, undefined, template.email?.sender_id).sender

  return (
    <div className="flex flex-col gap-3" data-email-preview>
      <KeyValueList
        labelWidth={70}
        items={[
          { key: 'from', label: t`From`, value: config.from_override || (sender ? formatSender(sender) : t`Default sender`) },
          { key: 'subject', label: t`Subject`, value: config.subject_override || template.email?.subject }
        ]}
      />
      <TemplatePreviewFrame workspace={workspace} template={template} />
      <div className="flex items-center gap-2">
        <TemplatePreviewPopover record={template} workspace={workspace}>
          <Button size="small" icon={<Eye size={14} aria-hidden="true" />}>
            {t`Preview`}
          </Button>
        </TemplatePreviewPopover>
        <Button
          size="small"
          icon={<ExternalLink size={14} aria-hidden="true" />}
          href={`/console/workspace/${workspace.id}/templates?preview=${encodeURIComponent(template.id)}`}
          target="_blank"
          rel="noopener noreferrer"
        >
          {t`Open template`}
        </Button>
      </div>
    </div>
  )
}
