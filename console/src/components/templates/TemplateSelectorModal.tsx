import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { Alert, Button, Dropdown, Input, Modal, Segmented, Select, Spin, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { useQuery } from '@tanstack/react-query'
import { CircleMinus, ExternalLink, Eye, Lock, Mail, MoreHorizontal, Plus, Search } from 'lucide-react'
import { templatesApi, type Template, type TemplateUsageReference } from '../../services/api/template'
import { templateLanguages } from './templateLanguages'
import type { Workspace } from '../../services/api/types'
import { resolveSenderInfo, type SendingProvider } from '../transactional/senderInfo'
import { TemplatePreviewFrame } from '../broadcasts/detail/TemplatePreviewFrame'
import { useTemplatePreview } from '../broadcasts/detail/useTemplatePreview'
import { Pill, useRelativeDate } from '../layout'
import { TemplateCategoryTag } from './TemplateCategoryTag'
import { TEMPLATE_CATEGORIES, TEMPLATE_CATEGORY_LABELS, isTemplateCategory } from './templateCategories'
import { groupReferences, referenceStatusLabel, summarizeUsage } from './templateUsage'
import { ownVariablePaths } from './liquidVariablePaths'
import { useLanguageName } from './useLanguageName'
import { LazyVisible } from './LazyVisible'
import { NotInUse } from './NotInUse'
import { buildDefaultTestData } from './defaultTestData'
import { GALLERY_MODELS } from './gallery/models'
import { MODEL_LABELS } from './gallery/labels'
import { useGalleryAssets, useGalleryModels } from './gallery/useGalleryModels'

// The "Any template" row's key among the templates' ids.
const ANY = '\u0000any'
// How long the pointer rests on a row before the preview follows it: crossing the list does not
// compile every row it passes over.
const HOVER_MS = 120
// The references "Used by" names per type; the rest is counted.
const REFERENCES_SHOWN = 3

export interface TemplateSelectorModalProps {
  workspace: Workspace
  /** The template chosen in the field, marked Current. */
  value?: string | null
  /** The category the caller imposes: the API lists only it, the filter is locked on it. */
  category?: string
  /** Set when an empty value means every template: a first row of that name chooses it. */
  anyLabel?: string
  /** What the template is for, under the title. */
  subtitle?: string
  /** Whether the member may create templates (Templates write). */
  canWrite: boolean
  /** An automation's Email node: says what the category changes for the automation's list. */
  listStatusNotice?: boolean
  /**
   * The provider that sends the chosen template, as its send path picks it: the preview names the
   * sender it gets there. Absent for a filter that sends nothing: no sender is shown.
   */
  sendsWith?: SendingProvider
  /** A sender that goes before the template's own: a broadcast variant's. */
  senderOverrideId?: string
  onCancel: () => void
  /** A template, or null for "Any template". */
  onPick: (template: Template | null) => void
  onNew: () => void
  onDuplicate: (template: Template) => void
  /** A gallery model, from the models shown when the imposed category has no template. */
  onStartModel: (modelId: string) => void
  /** The full preview, in the language the pane shows when it comes from there. */
  onFullPreview: (template: Template, language?: string) => void
}

/**
 * The window a template field opens: the workspace's email templates with a preview of the one
 * pointed at, in the gallery's frame. A click previews, a double click or "Use this template"
 * chooses.
 */
export function TemplateSelectorModal({
  workspace,
  value,
  category,
  anyLabel,
  subtitle,
  canWrite,
  listStatusNotice,
  sendsWith,
  senderOverrideId,
  onCancel,
  onPick,
  onNew,
  onDuplicate,
  onStartModel,
  onFullPreview
}: TemplateSelectorModalProps) {
  const { t, i18n } = useLingui()
  const relative = useRelativeDate()
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<string | undefined>(category)
  const [previewId, setPreviewId] = useState<string | null>(value || (anyLabel ? ANY : null))
  const hoverTimer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(hoverTimer.current), [])
  const currentRef = useRef<HTMLLIElement | null>(null)

  const list = useQuery({
    queryKey: ['templates', workspace.id, category ?? 'all', 'selector'],
    queryFn: () => templatesApi.list({ workspace_id: workspace.id, category, channel: 'email' })
  })
  // Every template's counts at once; a type the member cannot read is absent from them.
  const usage = useQuery({
    queryKey: ['templates-usage', workspace.id],
    queryFn: () => templatesApi.usage({ workspace_id: workspace.id })
  })

  const all = useMemo(() => list.data?.templates ?? [], [list.data])
  const query = search.trim().toLowerCase()
  const templates = all
    .filter((template) => !filter || template.category === filter)
    .filter(
      (template) =>
        !query ||
        template.name.toLowerCase().includes(query) ||
        (template.email?.subject ?? '').toLowerCase().includes(query)
    )
    .sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? ''))
  const filtering = Boolean(query) || filter !== category
  const showAny = Boolean(anyLabel) && !filtering

  // The current template in view once the list has loaded.
  useEffect(() => {
    if (list.isSuccess) currentRef.current?.scrollIntoView?.({ block: 'nearest' })
  }, [list.isSuccess])

  // Only a row on screen is previewed: a template a search or the category filter hides is not
  // left in the pane with "Use this template" still choosing it. Clearing them brings it back.
  const previewed = previewId === ANY ? (showAny ? ANY : undefined) : templates.find((template) => template.id === previewId)

  const point = (id: string) => {
    window.clearTimeout(hoverTimer.current)
    hoverTimer.current = window.setTimeout(() => setPreviewId(id), HOVER_MS)
  }
  const show = (id: string) => {
    window.clearTimeout(hoverTimer.current)
    setPreviewId(id)
  }
  const use = (id: string) => {
    window.clearTimeout(hoverTimer.current)
    if (id === ANY) onPick(null)
    else {
      const template = all.find((item) => item.id === id)
      if (template) onPick(template)
    }
  }

  // Up and down move between the rows.
  const onListKeyDown = (event: KeyboardEvent<HTMLUListElement>) => {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    const rows = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('[data-row]')]
    const index = rows.findIndex((row) => row === document.activeElement)
    if (index === -1) return
    event.preventDefault()
    rows[Math.max(0, Math.min(rows.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1)))]?.focus()
  }

  const noWriteReason = t`You don't have write permission for templates`
  const categoryLabel = (key: string) => (isTemplateCategory(key) ? i18n._(TEMPLATE_CATEGORY_LABELS[key]) : key)
  const languageCodes = (template: Template) => templateLanguages(template, workspace.settings?.default_language || 'en')
  const usageLabel = (template: Template) => {
    if (usage.isPending) return null
    if (usage.isError) return <span className="text-ink-4">—</span>
    const summary = summarizeUsage(usage.data.usage?.[template.id])
    return summary.label ? <span className="text-ink-2">{summary.label}</span> : <NotInUse />
  }

  const newButton = canWrite ? (
    <Button icon={<Plus size={14} aria-hidden="true" />} onClick={onNew}>
      {t`New template`}
    </Button>
  ) : (
    <Tooltip title={noWriteReason}>
      <span>
        <Button icon={<Plus size={14} aria-hidden="true" />} disabled>
          {t`New template`}
        </Button>
      </span>
    </Tooltip>
  )

  const menu = (template: Template): MenuProps['items'] => [
    { key: 'preview', icon: <ExternalLink size={14} aria-hidden="true" />, label: t`Full preview`, onClick: () => onFullPreview(template) },
    {
      key: 'duplicate',
      icon: <Plus size={14} aria-hidden="true" />,
      label: canWrite ? (
        t`Duplicate and edit`
      ) : (
        <Tooltip title={noWriteReason}>
          <span>{t`Duplicate and edit`}</span>
        </Tooltip>
      ),
      disabled: !canWrite,
      onClick: () => onDuplicate(template)
    }
  ]

  // The imposed category, or the whole workspace, has no template at all. A filter's "Any
  // template" is still a choice: its list shows that row alone.
  const nothingYet = list.isSuccess && all.length === 0 && !anyLabel

  const title = (
    <div className="flex items-start justify-between gap-3 pr-8">
      <div className="flex min-w-0 flex-col">
        <span>{t`Choose a template`}</span>
        {subtitle ? <span className="truncate text-sm font-normal text-ink-3">{subtitle}</span> : null}
      </div>
      {newButton}
    </div>
  )

  const footer = (
    <div className="flex items-center justify-between gap-3">
      <span className="text-left text-xs text-ink-3">
        {nothingYet && category && canWrite ? t`Models come from the template gallery.` : t`A click previews a template; a double click uses it.`}
      </span>
      <Button onClick={onCancel}>{t`Cancel`}</Button>
    </div>
  )

  const imposedLabel = category ? categoryLabel(category) : undefined

  let body: ReactNode
  if (list.isLoading) {
    body = (
      <div className="flex flex-1 items-center justify-center">
        <Spin size="small" />
      </div>
    )
  } else if (list.isError) {
    body = (
      <EmptyState icon={<Mail size={18} aria-hidden="true" />} title={t`Couldn't load the templates`}>
        <Button onClick={() => void list.refetch()}>{t`Retry`}</Button>
      </EmptyState>
    )
  } else if (nothingYet) {
    body = (
      <EmptyState
        icon={canWrite ? <Mail size={18} aria-hidden="true" /> : <Lock size={18} aria-hidden="true" />}
        title={imposedLabel ? t`No template in the ${imposedLabel} category yet` : t`No template yet`}
        description={
          canWrite
            ? category
              ? t`Start from a model: it opens in the template editor, and comes back selected here once saved.`
              : undefined
            : t`You need Templates write permission to create one. Ask a workspace owner to add one.`
        }
      >
        {canWrite && category ? <CategoryModels workspace={workspace} category={category} onStart={onStartModel} /> : newButton}
      </EmptyState>
    )
  } else {
    const shown = templates.length
    const total = all.length
    const count = shown === total ? plural(total, { one: '# template', other: '# templates' }) : t`${shown} of ${total} templates`
    const typed = search.trim()
    body = (
      <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_360px] gap-4">
        <div className="flex min-h-0 flex-col gap-2">
          <div className="flex gap-1.5 text-xs text-ink-3">
            <span>{count}</span>
            <span aria-hidden="true">·</span>
            <span className="truncate">{query ? t`Name or subject contains “${typed}”` : t`Last updated first`}</span>
          </div>
          {templates.length === 0 && !showAny ? (
            query ? (
              <EmptyState
                icon={<Search size={18} aria-hidden="true" />}
                title={t`No template matches “${typed}”`}
                description={t`Search looks in names and subjects.`}
              >
                <div className="flex gap-2">
                  <Button onClick={() => setSearch('')}>{t`Clear search`}</Button>
                  {newButton}
                </div>
              </EmptyState>
            ) : (
              <EmptyState icon={<Search size={18} aria-hidden="true" />} title={t`No template in this category`}>
                <Button onClick={() => setFilter(category)}>{t`All categories`}</Button>
              </EmptyState>
            )
          ) : (
            <ul
              aria-label={t`Templates`}
              onKeyDown={onListKeyDown}
              className="m-0 flex min-h-0 flex-1 list-none flex-col overflow-y-auto rounded-lg border border-line p-0"
            >
              {showAny ? (
                <li
                  ref={!value ? currentRef : undefined}
                  className={`flex items-center border-b border-line-soft ${previewId === ANY ? 'bg-primary-50' : ''}`}
                >
                  <RowButton
                    pressed={previewId === ANY}
                    onPoint={() => point(ANY)}
                    onShow={() => show(ANY)}
                    onUse={() => use(ANY)}
                  >
                    <span className="flex h-12 w-14 items-center justify-center rounded-sm bg-fill text-ink-3">
                      <CircleMinus size={18} aria-hidden="true" />
                    </span>
                    <span className="flex min-w-0 flex-col">
                      <span className="flex items-center gap-2">
                        <span className="truncate font-semibold text-ink">{anyLabel}</span>
                        {!value ? <CurrentPill /> : null}
                      </span>
                      <span className="truncate text-xs text-ink-3">{t`No filter on the template`}</span>
                    </span>
                  </RowButton>
                </li>
              ) : null}
              {templates.map((template) => {
                const name = template.name
                const current = template.id === value
                const pressed = template.id === previewId
                return (
                  <li
                    key={template.id}
                    ref={current ? currentRef : undefined}
                    className={`flex items-center gap-1 border-b border-line-soft pr-2 last:border-b-0 ${pressed ? 'bg-primary-50' : ''}`}
                  >
                    <RowButton
                      pressed={pressed}
                      onPoint={() => point(template.id)}
                      onShow={() => show(template.id)}
                      onUse={() => use(template.id)}
                    >
                      <span className="block h-12 w-14 overflow-hidden rounded-sm bg-fill">
                        <LazyVisible>
                          <TemplatePreviewFrame workspace={workspace} template={template} frameHeight={500} />
                        </LazyVisible>
                      </span>
                      <span className="flex min-w-0 flex-col">
                        <span className="flex min-w-0 items-center gap-2">
                          <span className="truncate font-semibold text-ink">{template.name}</span>
                          {current ? <CurrentPill /> : null}
                        </span>
                        <span className="truncate text-xs text-ink-3">{template.email?.subject}</span>
                      </span>
                      <span className="flex flex-col items-start gap-1">
                        <TemplateCategoryTag category={template.category} />
                        <span className="text-xs whitespace-nowrap text-ink-3">
                          {languageCodes(template)
                            .map((code) => code.toUpperCase())
                            .join(' · ')}
                        </span>
                      </span>
                      <span className="flex flex-col items-end gap-1 text-xs whitespace-nowrap">
                        {usageLabel(template)}
                        <span className="text-ink-3">{template.updated_at ? relative(template.updated_at) : null}</span>
                      </span>
                    </RowButton>
                    <Dropdown trigger={['click']} placement="bottomRight" menu={{ items: menu(template) }}>
                      <Button
                        type="text"
                        size="small"
                        aria-label={t`More actions for ${name}`}
                        icon={<MoreHorizontal size={16} aria-hidden="true" />}
                      />
                    </Dropdown>
                  </li>
                )
              })}
            </ul>
          )}
        </div>
        <section aria-label={t`Preview`} className="flex min-h-0 flex-col gap-3 rounded-lg border border-line p-3">
          {previewed === ANY ? (
            <>
              <h3 className="m-0 text-sm font-semibold text-ink">{anyLabel}</h3>
              <EmptyState
                icon={<CircleMinus size={18} aria-hidden="true" />}
                title={t`Every template`}
                description={t`No filter on the template: emails sent with any template match. Pick a template to narrow it.`}
              />
              <Button type="primary" block onClick={() => onPick(null)}>
                {t`Match any template`}
              </Button>
            </>
          ) : previewed ? (
            <TemplatePane
              key={previewed.id}
              workspace={workspace}
              template={previewed}
              usageTotal={usage.isSuccess ? summarizeUsage(usage.data.usage?.[previewed.id]).total : 0}
              listStatusNotice={listStatusNotice}
              sendsWith={sendsWith}
              senderOverrideId={senderOverrideId}
              onUse={() => onPick(previewed)}
              onFullPreview={(language) => onFullPreview(previewed, language)}
            />
          ) : (
            <EmptyState
              icon={<Eye size={18} aria-hidden="true" />}
              title={t`Pick a template to preview it`}
              description={t`This pane shows the template you point at, with its subject, sender and languages.`}
            />
          )}
        </section>
      </div>
    )
  }

  return (
    <Modal
      open
      onCancel={onCancel}
      title={title}
      width={960}
      footer={footer}
      centered
      // The gallery's frame: the window takes the screen's height, the list and the preview scroll.
      styles={{
        container: { height: 'calc(100vh - 48px)', display: 'flex', flexDirection: 'column' },
        body: { flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', gap: 12 }
      }}
    >
      <div className="flex gap-2">
        <div className="min-w-0 flex-1">
          <Input
            allowClear
            prefix={<Search size={14} aria-hidden="true" className="text-ink-3" />}
            placeholder={t`Search by name or subject`}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            aria-label={t`Search by name or subject`}
          />
        </div>
        <div className="w-56 shrink-0">
          {/* A disabled select sends no leave event: the span ends the tooltip when the pointer goes,
              instead of leaving it over the window's header. */}
          <Tooltip title={category ? t`The category is set by where this template is used` : undefined} placement="bottom">
            <span className="block">
              <Select
                style={{ width: '100%' }}
                allowClear={!category}
                disabled={Boolean(category)}
                prefix={category ? <Lock size={14} aria-hidden="true" /> : undefined}
                placeholder={t`All categories`}
                value={filter}
                onChange={(next: string | undefined) => setFilter(next)}
                options={TEMPLATE_CATEGORIES.map((key) => ({ value: key, label: i18n._(TEMPLATE_CATEGORY_LABELS[key]) }))}
                aria-label={t`Category`}
              />
            </span>
          </Tooltip>
        </div>
      </div>
      {body}
    </Modal>
  )
}

function CurrentPill() {
  const { t } = useLingui()
  return (
    <Pill tone="primary" dot={false}>
      {t({ message: 'Current', context: 'template selector' })}
    </Pill>
  )
}

/**
 * A row's main button: the pointer previews it after a rest, a click at once, a double click uses it.
 * The pointer counts once it moves: a window opening under a resting pointer (the field that opened
 * it sat there) gets an enter event in Chrome, and must not replace the current template's preview.
 */
function RowButton({
  pressed,
  onPoint,
  onShow,
  onUse,
  children
}: {
  pressed: boolean
  onPoint: () => void
  onShow: () => void
  onUse: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      data-row
      aria-pressed={pressed}
      onMouseMove={onPoint}
      onFocus={onShow}
      onClick={onShow}
      onDoubleClick={onUse}
      className="grid min-w-0 flex-1 cursor-pointer grid-cols-[56px_minmax(0,1fr)_auto_auto] items-center gap-3 border-0 bg-transparent px-3 py-2 text-left focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary-500"
    >
      {children}
    </button>
  )
}

function EmptyState({ icon, title, description, children }: { icon: ReactNode; title: string; description?: string; children?: ReactNode }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 px-6 py-8 text-center">
      <span className="flex size-10 items-center justify-center rounded-full bg-fill text-ink-3">{icon}</span>
      <h4 className="m-0 text-sm font-semibold text-ink">{title}</h4>
      {description ? <p className="m-0 max-w-md text-sm text-ink-2">{description}</p> : null}
      {children ? <div className="mt-2">{children}</div> : null}
    </div>
  )
}

/** The gallery's models of a category, and Blank: what an empty imposed category starts from. */
function CategoryModels({ workspace, category, onStart }: { workspace: Workspace; category: string; onStart: (modelId: string) => void }) {
  const { t, i18n } = useLingui()
  const assets = useGalleryAssets()
  const defaultTestData = useMemo(
    () => buildDefaultTestData(workspace.settings.custom_endpoint_url || window.API_ENDPOINT, workspace.id),
    [workspace.settings.custom_endpoint_url, workspace.id]
  )
  const { previewTemplate, logoReady } = useGalleryModels(workspace, assets, defaultTestData)
  const models = [
    ...GALLERY_MODELS.filter((model) => model.category === category),
    ...GALLERY_MODELS.filter((model) => model.id === 'blank')
  ]
  return (
    <ul aria-label={t`Models`} className="m-0 flex list-none flex-wrap justify-center gap-3 p-0">
      {models.map((model) => (
        <li key={model.id} className="w-44">
          <button
            type="button"
            data-model={model.id}
            onClick={() => onStart(model.id)}
            className="flex w-full cursor-pointer flex-col overflow-hidden rounded-lg border border-line bg-surface p-0 text-left transition-colors hover:border-primary-300 focus-visible:outline-2 focus-visible:outline-primary-500"
          >
            <span className="block h-32 overflow-hidden bg-fill px-3 pt-3">
              {model.id === 'blank' ? (
                <span className="flex h-full items-center justify-center">
                  <span className="flex size-9 items-center justify-center rounded-full border border-dashed border-ink-4 text-ink-3">
                    <Plus size={16} aria-hidden="true" />
                  </span>
                </span>
              ) : logoReady ? (
                <TemplatePreviewFrame workspace={workspace} template={previewTemplate(model)} frameHeight={560} />
              ) : null}
            </span>
            <span className="flex flex-col gap-1 border-t border-line-soft px-2.5 py-2">
              <span className="truncate text-sm font-semibold text-ink">{i18n._(MODEL_LABELS[model.id].name)}</span>
              <span className="flex h-5 items-center">
                {/* Blank takes the imposed category, as the template made from it will. */}
                <TemplateCategoryTag category={model.category ?? category} />
              </span>
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}

interface TemplatePaneProps {
  workspace: Workspace
  template: Template
  usageTotal: number
  listStatusNotice?: boolean
  sendsWith?: SendingProvider
  senderOverrideId?: string
  onUse: () => void
  /** Opens the full preview on the language shown. */
  onFullPreview: (language: string) => void
}

/** The previewed template: its render, desktop or mobile, in each of its languages, and what it needs. */
function TemplatePane({
  workspace,
  template,
  usageTotal,
  listStatusNotice,
  sendsWith,
  senderOverrideId,
  onUse,
  onFullPreview
}: TemplatePaneProps) {
  const { t, i18n } = useLingui()
  const languageName = useLanguageName()
  const [device, setDevice] = useState<'desktop' | 'mobile'>('desktop')
  const main = workspace.settings?.default_language || 'en'
  const languages = templateLanguages(template, main)
  const [language, setLanguage] = useState(main)
  // A translation as a template of its own, so its compile is cached apart from the main one.
  const shown: Template =
    language === main
      ? template
      : { ...template, id: `${template.id}@${language}`, email: template.translations?.[language]?.email }
  const compiled = useTemplatePreview(workspace, shown)
  const subject = compiled.data?.subject ?? shown.email?.subject
  const subjectPreview = compiled.data?.subject_preview ?? shown.email?.subject_preview
  const sender = sendsWith ? resolveSenderInfo(workspace, template, sendsWith, senderOverrideId).sender : null
  const variables = ownVariablePaths(template.test_data)
  const subscriptionSensitive = template.category === 'marketing' || template.category === 'blog'
  const categoryName = isTemplateCategory(template.category) ? i18n._(TEMPLATE_CATEGORY_LABELS[template.category]) : template.category

  return (
    <>
      <div className="flex items-center justify-between gap-2">
        <h3 className="m-0 truncate text-sm font-semibold text-ink">{template.name}</h3>
        <Segmented<'desktop' | 'mobile'>
          size="small"
          value={device}
          onChange={setDevice}
          options={[
            { value: 'desktop', label: t`Desktop` },
            { value: 'mobile', label: t`Mobile` }
          ]}
        />
      </div>
      {/* The email scrolls in its own box, so what it needs stays in view under it. */}
      <div className="flex min-h-40 flex-1 justify-center overflow-y-auto rounded-md bg-fill p-3">
        <div className={device === 'mobile' ? 'w-[240px] max-w-full' : 'w-full'}>
          <TemplatePreviewFrame workspace={workspace} template={shown} emailWidth={device === 'mobile' ? 375 : 600} frameHeight={1400} />
        </div>
      </div>
      <div className="flex max-h-[45%] shrink-0 flex-col gap-3 overflow-y-auto">
        {listStatusNotice ? (
          <Alert
            type="info"
            showIcon
            title={
              subscriptionSensitive
                ? t`${categoryName} template: in an automation tied to a list, contacts unsubscribed, bounced or complained on that list don't receive it and leave the automation.`
                : t`${categoryName} template: sent even to contacts unsubscribed, bounced or complained on the automation's list. Only Marketing and Blog templates skip them.`
            }
          />
        ) : null}
        <dl className="m-0 grid grid-cols-[88px_minmax(0,1fr)] gap-x-2 gap-y-1.5 text-sm">
          <dt className="text-ink-3">{t`Subject`}</dt>
          <dd className="m-0 break-words text-ink">{subject}</dd>
          {subjectPreview ? (
            <>
              <dt className="text-ink-3">{t`Preview text`}</dt>
              <dd className="m-0 break-words text-ink-2">{subjectPreview}</dd>
            </>
          ) : null}
          {sendsWith ? (
            <>
              <dt className="text-ink-3">{t`Sender`}</dt>
              {sender ? (
                <dd className="m-0 break-words text-ink-2">{sender.name ? `${sender.name} · ${sender.email}` : sender.email}</dd>
              ) : (
                <dd className="m-0 text-warning-fg">{t`No sender`}</dd>
              )}
            </>
          ) : null}
          <dt className="text-ink-3">{t`Language`}</dt>
          <dd className="m-0">
            {languages.length > 1 ? (
              <Segmented<string>
                size="small"
                value={language}
                onChange={setLanguage}
                options={languages.map((code) => ({ value: code, label: languageName(code) }))}
              />
            ) : (
              <span className="text-ink-2">{languageName(main)}</span>
            )}
          </dd>
          {usageTotal > 0 ? (
            <>
              <dt className="text-ink-3">{t`Used by`}</dt>
              <dd className="m-0">
                <UsedBy workspaceId={workspace.id} templateId={template.id} />
              </dd>
            </>
          ) : null}
        </dl>
        {variables.length > 0 ? (
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-semibold tracking-wide text-ink-3 uppercase">{t`Variables it uses`}</span>
            <div className="flex flex-wrap gap-1">
              {variables.map((variable) => (
                <code key={variable} className="rounded-sm bg-fill px-1.5 py-0.5 text-xs">
                  {variable}
                </code>
              ))}
            </div>
          </div>
        ) : null}
      </div>
      <div className="flex items-center justify-between gap-2">
        <Button type="link" icon={<ExternalLink size={14} aria-hidden="true" />} onClick={() => onFullPreview(language)}>
          {t`Full preview`}
        </Button>
        <Button type="primary" onClick={onUse}>
          {t`Use this template`}
        </Button>
      </div>
    </>
  )
}

/** What uses a template, by name: a few per type, the rest counted. */
function UsedBy({ workspaceId, templateId }: { workspaceId: string; templateId: string }) {
  const { t, i18n } = useLingui()
  const references = useQuery({
    queryKey: ['templates-usage', workspaceId, templateId],
    queryFn: () => templatesApi.usage({ workspace_id: workspaceId, template_id: templateId })
  })
  if (references.isPending) return <Spin size="small" />
  if (references.isError) return <span className="text-ink-4">—</span>
  const typeLabels: Record<TemplateUsageReference['type'], string> = {
    broadcast: t`Broadcast`,
    transactional_notification: t`Notification`,
    automation: t`Automation`,
    list: t`List`
  }
  const groups = groupReferences(references.data.references ?? [], references.data.usage?.[templateId], REFERENCES_SHOWN)
  return (
    <ul className="m-0 flex list-none flex-col gap-0.5 p-0">
      {groups.flatMap((group) => [
        ...group.items.map((item) => (
          <li key={`${item.type}-${item.id}`} className="min-w-0 break-words text-ink-2">
            {item.name}{' '}
            <span className="text-ink-3">
              ({item.status ? `${typeLabels[item.type]} · ${referenceStatusLabel(item.status, i18n)}` : typeLabels[item.type]})
            </span>
          </li>
        )),
        ...(group.more > 0
          ? [
              <li key={`${group.type}-more`} className="text-ink-3">
                {plural(group.more, { one: 'and # more', other: 'and # more' })}
              </li>
            ]
          : [])
      ])}
    </ul>
  )
}
