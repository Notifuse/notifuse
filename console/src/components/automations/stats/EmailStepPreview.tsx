import { Button, Skeleton } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { KeyValueList } from '../../layout'
import { templatesApi } from '../../../services/api/template'
import { ApiError } from '../../../services/api/errors'
import type { EmailNodeConfig } from '../../../services/api/automation'
import type { Workspace } from '../../../services/api/types'
import { TemplatePreviewFrame } from '../../broadcasts/detail/TemplatePreviewFrame'
import { formatSender, resolveVariationSender } from '../../broadcasts/variationSender'

/** The height of email shown in the step panel, at its 600px width: about 200px once scaled. */
export const STEP_PREVIEW_HEIGHT = 420

interface EmailStepPreviewProps {
  workspace: Workspace
  config: Partial<EmailNodeConfig>
  canReadTemplates: boolean
}

/** The sender, the subject and a render of the top of the step's template. */
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
  // A template deleted since is not a read that failed: only the second can be retried.
  if (query.isError && !(query.error instanceof ApiError && query.error.status === 404)) {
    return (
      <div data-unreset className="flex items-center gap-2 text-sm text-ink-3">
        <span>{t`Couldn't load the template`}</span>
        <Button size="small" onClick={() => void query.refetch()}>
          {t`Retry`}
        </Button>
      </div>
    )
  }
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
      {/* The top of the email, as a thumbnail: the panel's results stay in view under it. */}
      <TemplatePreviewFrame workspace={workspace} template={template} frameHeight={STEP_PREVIEW_HEIGHT} />
    </div>
  )
}
