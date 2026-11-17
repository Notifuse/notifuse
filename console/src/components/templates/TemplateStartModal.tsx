import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Button, Input, Modal, Segmented, Select, Spin } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Plus, Search } from 'lucide-react'
import { templatesApi, type Template } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import { TemplatePreviewFrame } from '../broadcasts/detail/TemplatePreviewFrame'
import { useRelativeDate } from '../layout'
import { TemplateCategoryTag } from './TemplateCategoryTag'
import { TEMPLATE_CATEGORIES, TEMPLATE_CATEGORY_LABELS, isTemplateCategory } from './templateCategories'
import { ownVariablePaths } from './liquidVariablePaths'
import { GALLERY_MODELS } from './gallery/models'
import type { GalleryStart } from './gallery/buildStartingPoint'
import { useGalleryModels } from './gallery/useGalleryModels'
import { LazyVisible } from './LazyVisible'
import { GALLERY_USE_CASES, type GalleryAssets, type GalleryModel, type GalleryUseCase } from './gallery/types'
import { MODEL_LABELS, SENT_BY_LABELS, USE_CASE_LABELS } from './gallery/labels'

// How long a click on a card waits for a second one before opening the preview.
const DOUBLE_CLICK_MS = 250

export interface TemplateStartModalProps {
  open: boolean
  workspace: Workspace
  assets: GalleryAssets
  defaultTestData: Record<string, unknown>
  /** The category the template must have, imposed by where it is created. */
  forceCategory?: string
  /** Whether the member may read templates, which "Your templates" lists. */
  canReadTemplates: boolean
  onCancel: () => void
  onPickModel: (start: GalleryStart) => void
  onPickTemplate: (template: Template) => void
}

type View = { kind: 'grid' } | { kind: 'model'; model: GalleryModel } | { kind: 'template'; template: Template }

/**
 * Where a new template starts: a model of the gallery, one of the workspace's templates, or
 * Blank. The template drawer opens after it, filled with the choice.
 */
export function TemplateStartModal({
  open,
  workspace,
  assets,
  defaultTestData,
  forceCategory,
  canReadTemplates,
  onCancel,
  onPickModel,
  onPickTemplate
}: TemplateStartModalProps) {
  const { t, i18n } = useLingui()
  const [tab, setTab] = useState<'gallery' | 'mine'>('gallery')
  const [useCase, setUseCase] = useState<GalleryUseCase | 'all'>('all')
  const [search, setSearch] = useState('')
  const [view, setView] = useState<View>({ kind: 'grid' })
  const clickTimer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(clickTimer.current), [])

  // Its languages, its name instead of the example brand, its logo instead of the placeholder,
  // sized once the logo has loaded; the previews wait for that size, so each compiles once.
  const gallery = useGalleryModels(workspace, assets, defaultTestData)
  const { previewTemplate, logoReady } = gallery
  const start = async (model: GalleryModel) => onPickModel(await gallery.start(model))

  const name = (model: GalleryModel) => i18n._(MODEL_LABELS[model.id].name)
  const query = search.trim().toLowerCase()
  const models = GALLERY_MODELS.filter((model) => {
    if (model.id === 'blank') return true
    if (forceCategory && model.category !== forceCategory) return false
    if (!forceCategory && useCase !== 'all' && model.useCase !== useCase) return false
    return !query || name(model).toLowerCase().includes(query) || i18n._(MODEL_LABELS[model.id].description).toLowerCase().includes(query)
  })
  const counts = Object.fromEntries(
    GALLERY_USE_CASES.map((key) => [key, GALLERY_MODELS.filter((model) => model.useCase === key).length])
  ) as Record<GalleryUseCase, number>
  const total = GALLERY_MODELS.filter((model) => model.id !== 'blank').length
  const forcedLabel = forceCategory && isTemplateCategory(forceCategory) ? i18n._(TEMPLATE_CATEGORY_LABELS[forceCategory]) : forceCategory

  const showMine = canReadTemplates
  const title = (
    <div className="flex items-center justify-between gap-3 pr-8">
      <span>{t`New template`}</span>
      {showMine && view.kind === 'grid' ? (
        <Segmented<'gallery' | 'mine'>
          size="small"
          value={tab}
          onChange={setTab}
          options={[
            { value: 'gallery', label: t`Gallery` },
            { value: 'mine', label: t`Your templates` }
          ]}
        />
      ) : null}
    </div>
  )

  return (
    <Modal
      open={open}
      onCancel={onCancel}
      title={title}
      width={960}
      footer={null}
      destroyOnHidden
      centered
      // The window takes the screen's height and never scrolls itself: each view scrolls its
      // own list or preview, the header, filters and search staying in place.
      styles={{
        container: { height: 'calc(100vh - 48px)', display: 'flex', flexDirection: 'column' },
        body: { flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }
      }}
    >
      {view.kind === 'model' ? (
        <StartPreview
          workspace={workspace}
          template={previewTemplate(view.model)}
          backLabel={t`Gallery`}
          onBack={() => setView({ kind: 'grid' })}
          title={name(view.model)}
          description={i18n._(MODEL_LABELS[view.model.id].description)}
          category={view.model.category ?? forceCategory}
          sentBy={i18n._(SENT_BY_LABELS[view.model.sentBy])}
          // The variables a model needs on top of those every send fills in.
          variables={ownVariablePaths(previewTemplate(view.model).test_data as Record<string, unknown>)}
          action={t`Use this model`}
          onUse={() => start(view.model)}
        />
      ) : view.kind === 'template' ? (
        <StartPreview
          workspace={workspace}
          template={view.template}
          backLabel={t`Your templates`}
          onBack={() => setView({ kind: 'grid' })}
          title={view.template.name}
          category={view.template.category}
          action={t`Start from this template`}
          onUse={() => onPickTemplate(view.template)}
        />
      ) : tab === 'mine' && showMine ? (
        <YourTemplates workspace={workspace} forceCategory={forceCategory} onPreview={(template) => setView({ kind: 'template', template })} onPick={onPickTemplate} />
      ) : (
        <div
          className={
            forceCategory
              ? 'flex min-h-0 flex-1 flex-col gap-3'
              : 'grid min-h-0 flex-1 grid-cols-[180px_minmax(0,1fr)] gap-4'
          }
        >
          {forceCategory ? (
            <div className="rounded-md bg-primary-50 px-3 py-2 text-sm text-primary-700">
              {t`The category ${forcedLabel} is imposed where this template is created: the models of other categories are hidden.`}
            </div>
          ) : (
            <nav aria-label={t`Use cases`} className="flex flex-col gap-0.5">
              <UseCaseButton active={useCase === 'all'} onClick={() => setUseCase('all')} count={total}>
                {t`All`}
              </UseCaseButton>
              {GALLERY_USE_CASES.map((key) => (
                <UseCaseButton key={key} active={useCase === key} onClick={() => setUseCase(key)} count={counts[key]}>
                  {i18n._(USE_CASE_LABELS[key])}
                </UseCaseButton>
              ))}
            </nav>
          )}
          <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3">
            <Input
              allowClear
              prefix={<Search size={14} aria-hidden="true" className="text-ink-3" />}
              placeholder={t`Search a model`}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              aria-label={t`Search a model`}
            />
            <ul className="m-0 grid min-h-0 flex-1 list-none auto-rows-min grid-cols-4 gap-3 overflow-y-auto p-0 pr-1" aria-label={t`Models`}>
              {models.map((model) => (
                <li key={model.id}>
                  <button
                    type="button"
                    data-model={model.id}
                    className="flex w-full cursor-pointer flex-col overflow-hidden rounded-lg border border-line bg-surface p-0 text-left transition-colors hover:border-primary-300 focus-visible:outline-2 focus-visible:outline-primary-500"
                    onClick={() => {
                      if (model.id === 'blank') return start(model)
                      // Wait for a possible second click: a double click uses the model.
                      window.clearTimeout(clickTimer.current)
                      clickTimer.current = window.setTimeout(() => setView({ kind: 'model', model }), DOUBLE_CLICK_MS)
                    }}
                    onDoubleClick={() => {
                      window.clearTimeout(clickTimer.current)
                      if (model.id !== 'blank') start(model)
                    }}
                  >
                    <div className="h-36 overflow-hidden bg-fill px-3 pt-3">
                      {model.id === 'blank' ? (
                        <div className="flex h-full items-center justify-center">
                          <span className="flex size-9 items-center justify-center rounded-full border border-dashed border-ink-4 text-ink-3">
                            <Plus size={16} aria-hidden="true" />
                          </span>
                        </div>
                      ) : (
                        <LazyVisible>
                          {logoReady ? (
                            <TemplatePreviewFrame workspace={workspace} template={previewTemplate(model)} frameHeight={560} />
                          ) : null}
                        </LazyVisible>
                      )}
                    </div>
                    <div className="flex flex-col gap-1 border-t border-line-soft px-2.5 py-2">
                      <span className="truncate text-sm font-semibold text-ink">{name(model)}</span>
                      {/* The height of a category tag, so Blank's card lines up with the others. */}
                      <span className="flex h-5 items-center">
                        {/* Blank has no category of its own: an imposed one is the one it gets. */}
                        {model.category || forceCategory ? (
                          <TemplateCategoryTag category={model.category ?? forceCategory ?? ''} />
                        ) : (
                          <span className="text-xs text-ink-3">{t`Category to choose`}</span>
                        )}
                      </span>
                    </div>
                  </button>
                </li>
              ))}
            </ul>
            <p className="m-0 text-xs text-ink-3">{t`A click opens the preview; a double click uses the model straight away.`}</p>
          </div>
        </div>
      )}
    </Modal>
  )
}

function UseCaseButton({ active, onClick, count, children }: { active: boolean; onClick: () => void; count: number; children: ReactNode }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={`flex cursor-pointer items-center justify-between gap-2 rounded-md border-0 px-2 py-1.5 text-left text-sm ${
        active ? 'bg-primary-100 font-semibold text-primary-700' : 'bg-transparent text-ink-2 hover:bg-fill'
      }`}
    >
      <span>{children}</span>
      <span className="text-xs text-ink-3 tabular-nums">{count}</span>
    </button>
  )
}

interface StartPreviewProps {
  workspace: Workspace
  template: Template
  backLabel: string
  onBack: () => void
  title: string
  description?: string
  category?: string
  sentBy?: string
  variables?: string[]
  action: string
  onUse: () => void
}

/** A model or a template at full size, desktop or mobile, with what it needs. */
function StartPreview({ workspace, template, backLabel, onBack, title, description, category, sentBy, variables, action, onUse }: StartPreviewProps) {
  const { t } = useLingui()
  const [device, setDevice] = useState<'desktop' | 'mobile'>('desktop')

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <Button type="text" size="small" icon={<ArrowLeft size={14} aria-hidden="true" />} onClick={onBack}>
          {backLabel}
        </Button>
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
      <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_260px] gap-4">
        <div className="flex min-h-0 justify-center overflow-y-auto rounded-lg bg-fill p-4">
          <div className={device === 'mobile' ? 'w-[375px] max-w-full' : 'w-full max-w-[600px]'}>
            <TemplatePreviewFrame workspace={workspace} template={template} emailWidth={device === 'mobile' ? 375 : 600} frameHeight={1400} />
          </div>
        </div>
        <aside className="flex min-h-0 flex-col gap-3 overflow-y-auto">
          <h3 className="m-0 text-subtitle font-semibold text-ink">{title}</h3>
          {description ? <p className="m-0 text-ink-2">{description}</p> : null}
          <dl className="m-0 grid grid-cols-[80px_minmax(0,1fr)] gap-x-2 gap-y-1.5 text-sm">
            <dt className="text-ink-3">{t`Category`}</dt>
            <dd className="m-0">{category ? <TemplateCategoryTag category={category} /> : t`To choose`}</dd>
            <dt className="text-ink-3">{t`Subject`}</dt>
            <dd className="m-0 break-words">{template.email?.subject}</dd>
            {sentBy ? (
              <>
                <dt className="text-ink-3">{t`Sent by`}</dt>
                <dd className="m-0">{sentBy}</dd>
              </>
            ) : null}
          </dl>
          {variables && variables.length > 0 ? (
            <div className="flex flex-col gap-1.5">
              <span className="text-xs font-semibold tracking-wide text-ink-3 uppercase">{t`Variables it uses`}</span>
              <div className="flex flex-wrap gap-1">
                {variables.map((variable) => (
                  <code key={variable} className="rounded-sm bg-fill px-1.5 py-0.5 text-xs">
                    {variable}
                  </code>
                ))}
              </div>
              <p className="m-0 text-xs text-ink-3">{t`Added to the template's test data. A send has to provide them.`}</p>
            </div>
          ) : null}
          <div className="flex-1" />
          <Button type="primary" block onClick={onUse}>
            {action}
          </Button>
        </aside>
      </div>
    </div>
  )
}

/** The workspace's templates, to start a new one from a copy. */
function YourTemplates({
  workspace,
  forceCategory,
  onPreview,
  onPick
}: {
  workspace: Workspace
  forceCategory?: string
  onPreview: (template: Template) => void
  onPick: (template: Template) => void
}) {
  const { t, i18n } = useLingui()
  const relative = useRelativeDate()
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<string | undefined>(forceCategory)
  const list = useQuery({
    queryKey: ['templates', workspace.id, forceCategory ?? 'all', 'start-modal'],
    queryFn: () => templatesApi.list({ workspace_id: workspace.id, category: forceCategory, channel: 'email' })
  })

  const query = search.trim().toLowerCase()
  const templates = (list.data?.templates ?? [])
    .filter((template) => !category || template.category === category)
    .filter((template) => !query || template.name.toLowerCase().includes(query) || (template.email?.subject ?? '').toLowerCase().includes(query))
    .sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? ''))

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex gap-2">
        <Input
          allowClear
          prefix={<Search size={14} aria-hidden="true" className="text-ink-3" />}
          placeholder={t`Search your templates`}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          aria-label={t`Search your templates`}
        />
        <Select
          className="w-56 shrink-0"
          allowClear={!forceCategory}
          disabled={!!forceCategory}
          placeholder={t`All categories`}
          value={category}
          onChange={setCategory}
          options={TEMPLATE_CATEGORIES.map((value) => ({ value, label: i18n._(TEMPLATE_CATEGORY_LABELS[value]) }))}
          aria-label={t`Category`}
        />
      </div>
      {list.isLoading ? (
        <div className="flex h-40 items-center justify-center">
          <Spin size="small" />
        </div>
      ) : templates.length === 0 ? (
        <p className="m-0 py-10 text-center text-ink-3">{t`No template matches.`}</p>
      ) : (
        <ul className="m-0 flex min-h-0 flex-1 list-none flex-col overflow-y-auto rounded-lg border border-line p-0">
          {templates.map((template) => (
            <li key={template.id} className="grid grid-cols-[56px_minmax(0,1fr)_auto_auto_auto] items-center gap-3 border-b border-line-soft px-3 py-2 last:border-b-0">
              <div className="h-12 w-14 overflow-hidden rounded-sm bg-fill">
                <LazyVisible>
                  <TemplatePreviewFrame workspace={workspace} template={template} frameHeight={500} />
                </LazyVisible>
              </div>
              <div className="flex min-w-0 flex-col">
                <span className="truncate font-semibold text-ink">{template.name}</span>
                <span className="truncate text-xs text-ink-3">{template.email?.subject}</span>
              </div>
              <TemplateCategoryTag category={template.category} />
              <span className="text-xs whitespace-nowrap text-ink-3">{template.updated_at ? relative(template.updated_at) : null}</span>
              <span className="flex gap-1">
                <Button size="small" type="text" onClick={() => onPreview(template)}>
                  {t`Preview`}
                </Button>
                <Button size="small" onClick={() => onPick(template)}>
                  {t`Start from this template`}
                </Button>
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
