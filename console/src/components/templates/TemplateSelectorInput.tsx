import React, { useState, useEffect } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Input, Button, Space, Tooltip, Skeleton } from 'antd'
import { EyeOutlined, LockOutlined } from '@ant-design/icons'
import { ChevronDown, Mail, TriangleAlert } from 'lucide-react'
import { templatesApi } from '../../services/api/template'
import type { Template } from '../../services/api/types'
import TemplatePreviewPopover from './TemplatePreviewDrawer'
import { CreateTemplateDrawer } from './CreateTemplateDrawer'
import { TemplateSelectorModal } from './TemplateSelectorModal'
import { useAuth, useWorkspacePermissions } from '../../contexts/AuthContext'
import type { SendingProvider } from '../transactional/senderInfo'
import { TemplatePreviewFrame } from '../broadcasts/detail/TemplatePreviewFrame'
import { TEMPLATE_CATEGORY_LABELS, isTemplateCategory } from './templateCategories'

interface TemplateSelectorInputProps {
  value?: string | null
  onChange?: (value: string | null) => void
  workspaceId: string
  category?:
    | 'marketing'
    | 'transactional'
    | 'welcome'
    | 'opt_in'
    | 'unsubscribe'
    | 'bounce'
    | 'blocklist'
    | 'other'
    | 'blog'
  placeholder?: string
  clearable?: boolean
  disabled?: boolean
  size?: 'small' | 'middle' | 'large'
  // Without Templates read, templates.list and templates.get answer 403: the field then shows
  // the stored template ID, requests nothing, cannot be changed, and a lock says why. Defaults
  // to true.
  canReadTemplates?: boolean
  /**
   * For a filter where an empty value means every template (a segment condition, a trigger): the
   * window lists a first row of this name, and choosing it empties the value, as the clear cross
   * does. Needs `clearable`.
   */
  anyLabel?: string
  /** What the template is for, under the window's title ("Email template of Password Reset"). */
  subtitle?: string
  /**
   * An automation's Email node: the preview says whether the template's category skips the
   * contacts unsubscribed, bounced or complained on the automation's list.
   */
  listStatusNotice?: boolean
  /**
   * The provider that will send the chosen template, as the caller's send path picks it: the
   * window shows the sender the template gets there. Left out by a filter that sends nothing (a
   * segment condition, a trigger): the window shows no sender.
   */
  sendsWith?: SendingProvider
  /** A sender set by the caller that goes before the template's own: a broadcast variant's. */
  senderOverrideId?: string
  /**
   * `card` shows the chosen template as a card (thumbnail, name, category and sender line, Preview
   * and Change) instead of the input; the window, the editor and the full preview are the same.
   */
  variant?: 'input' | 'card'
}

// What the editor opened from the window starts from.
type Creation = { kind: 'new' } | { kind: 'duplicate'; template: Template } | { kind: 'model'; modelId: string }

const TemplateSelectorInput: React.FC<TemplateSelectorInputProps> = ({
  value,
  onChange,
  workspaceId,
  category,
  placeholder,
  clearable = true,
  disabled = false,
  size,
  canReadTemplates = true,
  anyLabel,
  subtitle,
  listStatusNotice,
  sendsWith,
  senderOverrideId,
  variant = 'input'
}) => {
  const { t, i18n } = useLingui()
  const defaultPlaceholder = placeholder || t`Select a template`
  const [open, setOpen] = useState<boolean>(false)
  const [creation, setCreation] = useState<Creation | null>(null)
  const [fullPreview, setFullPreview] = useState<{ template: Template; language?: string } | null>(null)
  const [selectedTemplate, setSelectedTemplate] = useState<Template | null>(null)
  // The value templates.get could not resolve (deleted, or the request failed): the card says so
  // instead of loading for ever.
  const [unresolved, setUnresolved] = useState<string | null>(null)
  const { workspaces } = useAuth()
  // Creating and duplicating need Templates write; the editor manages the saved blocks with it too,
  // as the templates page does.
  const { permissions } = useWorkspacePermissions(workspaceId)
  const canWrite = Boolean(permissions?.templates?.write)

  // Find the current workspace from the workspaces array
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)

  // Keep the displayed template in sync with the controlled `value` prop.
  // The same component instance can be reused while `value` changes — e.g. switching
  // between email nodes in the automation editor — so we must re-resolve the template
  // whenever `value` no longer matches what we're showing, not just on first load.
  // What the field shows is the resolved template only while it is the value: a cleared or
  // changed value never shows the previous one.
  const shownTemplate = value && selectedTemplate?.id === value ? selectedTemplate : null
  useEffect(() => {
    // Nothing to resolve, or already resolved
    if (!value || selectedTemplate?.id === value) return
    if (!workspaceId || !canReadTemplates) return

    // Guard against out-of-order responses when `value` changes rapidly
    let cancelled = false
    templatesApi
      .get({ workspace_id: workspaceId, id: value })
      .then((response) => {
        if (!cancelled && response.template) {
          setSelectedTemplate(response.template)
        }
      })
      .catch((error) => {
        console.error('Failed to fetch template details:', error)
        if (!cancelled) setUnresolved(value)
      })
    return () => {
      cancelled = true
    }
  }, [value, workspaceId, selectedTemplate, canReadTemplates])

  const choose = (template: Template | null) => {
    setSelectedTemplate(template)
    onChange?.(template ? template.id : null)
    setOpen(false)
  }

  const showSelector = () => {
    // The input is disabled without Templates read; the check keeps the window shut anyway.
    if (!disabled && canReadTemplates) {
      setOpen(true)
    }
  }

  // The window closes while the gallery and the editor are open: never three layers over the form.
  const create = (next: Creation) => {
    setOpen(false)
    setCreation(next)
  }

  // Saved: the new template is chosen and the form is back. Cancelled: back to the window,
  // nothing chosen.
  const creationClosed = (saved?: Template) => {
    setCreation(null)
    if (saved) choose(saved)
    else setOpen(true)
  }

  if (!currentWorkspace) {
    return <div>{t`Loading...`}</div>
  }

  const lockHint = value
    ? t`You need Templates read permission to see this template's name and preview`
    : t`You need Templates read permission to choose a template`

  // The window, the full preview and the editor: the same whichever way the field is shown.
  const layers = (
    <>
      {open ? (
        <TemplateSelectorModal
          workspace={currentWorkspace}
          value={value}
          category={category}
          anyLabel={clearable ? anyLabel : undefined}
          subtitle={subtitle}
          canWrite={canWrite}
          listStatusNotice={listStatusNotice}
          sendsWith={sendsWith}
          senderOverrideId={senderOverrideId}
          onCancel={() => setOpen(false)}
          onPick={choose}
          onNew={() => create({ kind: 'new' })}
          onDuplicate={(template) => create({ kind: 'duplicate', template })}
          onStartModel={(modelId) => create({ kind: 'model', modelId })}
          onFullPreview={(template, language) => setFullPreview({ template, language })}
        />
      ) : null}

      {fullPreview ? (
        <TemplatePreviewPopover
          record={fullPreview.template}
          workspace={currentWorkspace}
          initialLanguage={fullPreview.language}
          open
          onClose={() => setFullPreview(null)}
        />
      ) : null}

      {creation ? (
        <CreateTemplateDrawer
          open
          workspace={currentWorkspace}
          canManageSavedBlocks={canWrite}
          forceCategory={category}
          fromTemplate={creation.kind === 'duplicate' ? creation.template : undefined}
          startModelId={creation.kind === 'model' ? creation.modelId : undefined}
          successMessage={t`Template created and selected`}
          onClose={creationClosed}
        />
      ) : null}
    </>
  )

  if (variant === 'card') {
    const categoryLabel = category && isTemplateCategory(category) ? i18n._(TEMPLATE_CATEGORY_LABELS[category]) : null
    const providerLabel =
      sendsWith === 'marketing'
        ? t`sent by the marketing provider`
        : sendsWith === 'transactional'
          ? t`sent by the transactional provider`
          : null
    return (
      <>
        <TemplateCard
          value={value}
          template={shownTemplate}
          notFound={!!value && unresolved === value && !shownTemplate}
          locked={!canReadTemplates}
          disabled={disabled}
          categoryLine={[categoryLabel ? t`${categoryLabel} category` : null, providerLabel].filter(Boolean).join(' · ')}
          emptyTitle={defaultPlaceholder}
          lockHint={
            value
              ? t`You need Templates read permission to see its name and preview`
              : t`You need Templates read permission to choose a template`
          }
          onChange={showSelector}
          preview={
            shownTemplate ? (
              <TemplatePreviewPopover record={shownTemplate} workspace={currentWorkspace}>
                <Button size="small">{t`Preview`}</Button>
              </TemplatePreviewPopover>
            ) : null
          }
          thumbnail={
            shownTemplate ? (
              <TemplatePreviewFrame workspace={currentWorkspace} template={shownTemplate} frameHeight={400} />
            ) : null
          }
        />
        {layers}
      </>
    )
  }

  return (
    <>
      <Space.Compact style={{ width: '100%' }}>
        <Input
          value={canReadTemplates ? shownTemplate?.name || '' : value || ''}
          placeholder={defaultPlaceholder}
          // Not read-only when clearable: a read-only input hides its clear cross.
          readOnly={!clearable}
          disabled={disabled || !canReadTemplates}
          onClick={showSelector}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ' || event.key === 'ArrowDown') {
              event.preventDefault()
              showSelector()
            }
          }}
          onClear={() => choose(null)}
          allowClear={clearable && canReadTemplates}
          suffix={
            canReadTemplates ? (
              <ChevronDown size={14} aria-hidden="true" className="text-ink-3" />
            ) : (
              // Focusable, so the hint also reaches keyboard users: the input itself is disabled.
              <Tooltip title={lockHint} trigger={['hover', 'focus']}>
                <LockOutlined aria-label={lockHint} tabIndex={0} />
              </Tooltip>
            )
          }
          size={size}
          style={{ flex: 1 }}
        />
        {canReadTemplates && shownTemplate && currentWorkspace && (
          <TemplatePreviewPopover record={shownTemplate} workspace={currentWorkspace}>
            <Button icon={<EyeOutlined />} size={size} aria-label={t`Preview`} />
          </TemplatePreviewPopover>
        )}
      </Space.Compact>

      {layers}
    </>
  )
}

interface TemplateCardProps {
  value?: string | null
  template: Template | null
  notFound: boolean
  locked: boolean
  disabled: boolean
  categoryLine: string
  emptyTitle: string
  lockHint: string
  preview: React.ReactNode
  thumbnail: React.ReactNode
  onChange: () => void
}

// The chosen template as a card: a thumbnail of its top, its name, what kind of email it is and
// who sends it. Without a value, an empty card that opens the window.
function TemplateCard({
  value,
  template,
  notFound,
  locked,
  disabled,
  categoryLine,
  emptyTitle,
  lockHint,
  preview,
  thumbnail,
  onChange
}: TemplateCardProps) {
  const { t } = useLingui()
  const canChange = !locked && !disabled

  let title: React.ReactNode
  let line: React.ReactNode = categoryLine
  if (locked) {
    title = value ? (
      <span className="font-mono text-sm text-ink">{value}</span>
    ) : (
      <span className="text-ink-3">{emptyTitle}</span>
    )
    line = lockHint
  } else if (!value) {
    title = <span className="font-medium text-ink">{emptyTitle}</span>
  } else if (template) {
    title = <span className="truncate font-medium text-ink">{template.name}</span>
  } else if (notFound) {
    title = <span className="font-mono text-sm text-ink">{value}</span>
    line = (
      <span className="inline-flex items-center gap-1 text-warning-fg">
        <TriangleAlert size={12} aria-hidden="true" />
        {t`This template cannot be found`}
      </span>
    )
  } else {
    title = <Skeleton.Input active size="small" style={{ width: 160, height: 16 }} />
  }

  return (
    <div
      data-template-card
      className={`flex items-center gap-3 rounded-md border p-2.5 ${value || locked ? 'border-line' : 'border-dashed border-line'}`}
    >
      {/* A fixed box that clips the frame: the top of the email, scaled to the box's width. */}
      <div className="flex h-16 w-24 shrink-0 items-center justify-center overflow-hidden rounded-sm border border-line-soft bg-subtle text-ink-4">
        {locked ? (
          <LockOutlined aria-hidden="true" />
        ) : thumbnail ? (
          <div className="w-full self-start">{thumbnail}</div>
        ) : (
          <Mail size={18} aria-hidden="true" />
        )}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex min-w-0">{title}</div>
        {line ? <div className="text-xs text-ink-3">{line}</div> : null}
      </div>
      {canChange ? (
        <div className="flex shrink-0 items-center gap-1.5">
          {preview}
          <Button size="small" onClick={onChange}>
            {value ? t`Change` : t`Choose`}
          </Button>
        </div>
      ) : null}
    </div>
  )
}

export default TemplateSelectorInput
