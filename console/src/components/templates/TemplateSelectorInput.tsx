import React, { useState, useEffect } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Input, Button, Space, Tooltip } from 'antd'
import { EyeOutlined, LockOutlined } from '@ant-design/icons'
import { ChevronDown } from 'lucide-react'
import { templatesApi } from '../../services/api/template'
import type { Template } from '../../services/api/types'
import TemplatePreviewPopover from './TemplatePreviewDrawer'
import { CreateTemplateDrawer } from './CreateTemplateDrawer'
import { TemplateSelectorModal } from './TemplateSelectorModal'
import { useAuth, useWorkspacePermissions } from '../../contexts/AuthContext'
import type { SendingProvider } from '../transactional/senderInfo'

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
  senderOverrideId
}) => {
  const { t } = useLingui()
  const defaultPlaceholder = placeholder || t`Select a template`
  const [open, setOpen] = useState<boolean>(false)
  const [creation, setCreation] = useState<Creation | null>(null)
  const [fullPreview, setFullPreview] = useState<Template | null>(null)
  const [selectedTemplate, setSelectedTemplate] = useState<Template | null>(null)
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
          onFullPreview={setFullPreview}
        />
      ) : null}

      {fullPreview ? (
        <TemplatePreviewPopover record={fullPreview} workspace={currentWorkspace} open onClose={() => setFullPreview(null)} />
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
}

export default TemplateSelectorInput
