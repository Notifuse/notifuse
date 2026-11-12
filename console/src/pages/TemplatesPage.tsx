import { useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Dropdown, Empty, Result, Skeleton, Table, Tooltip } from 'antd'
import type { MenuProps, TableColumnType } from 'antd'
import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { MoreHorizontal, Plus, SquareTerminal, Upload } from 'lucide-react'
import { templatesApi } from '../services/api/template'
import { ApiError } from '../services/api/errors'
import type { Template, Workspace } from '../services/api/types'
import { canSendTestEmail } from '../services/api/permissions'
import { CreateTemplateDrawer } from '../components/templates/CreateTemplateDrawer'
import TemplatePreviewDrawer from '../components/templates/TemplatePreviewDrawer'
import SendTemplateModal from '../components/templates/SendTemplateModal'
import { DeleteTemplateModal } from '../components/templates/DeleteTemplateModal'
import { ImportErrorsModal } from '../components/templates/ImportErrorsModal'
import {
  importTemplateFile,
  templateNameFromFile,
  type TemplateFileKind
} from '../components/templates/importTemplateFile'
import {
  TEMPLATE_CATEGORIES,
  TEMPLATE_CATEGORY_LABELS,
  isTemplateCategory,
  type TemplateCategory
} from '../components/templates/templateCategories'
import { summarizeUsage } from '../components/templates/templateUsage'
import type { EmailBlock } from '../components/email_builder/types'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { copyToClipboard } from '../lib/copyToClipboard'
import type { TemplatesSearch } from '../lib/searchParams'
import dayjs from '../lib/dayjs'
import {
  CountTabs,
  PageHeader,
  Panel,
  Pill,
  TableToolbar,
  useRelativeDate,
  type CountTabOption
} from '../components/layout'

const ROUTE = '/console/workspace/$workspaceId/templates'

/** How the create/edit drawer was opened. */
type DrawerOpening =
  | { mode: 'create' }
  | { mode: 'edit'; template: Template }
  | { mode: 'clone'; template: Template }
  | { mode: 'import'; name: string; tree: EmailBlock; testData?: Record<string, unknown> }

/** A new key remounts the drawer fresh each time it opens. */
type DrawerState = DrawerOpening & { key: number }

const integrationIcon = (type: string) =>
  type === 'supabase' ? (
    <img src="/console/supabase.png" alt="Supabase" className="h-3" />
  ) : (
    <SquareTerminal size={14} aria-hidden="true" className="text-ink-3" />
  )

export function TemplatesPage() {
  const { t, i18n } = useLingui()
  const { message, modal } = App.useApp()
  const relative = useRelativeDate()
  const { workspaceId } = useParams({ from: ROUTE })
  const search = useSearch({ from: ROUTE }) as TemplatesSearch
  const navigate = useNavigate({ from: ROUTE })
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const workspace: Workspace | undefined = workspaces.find((w) => w.id === workspaceId)
  const canWrite = Boolean(permissions?.templates?.write)
  const canTest = canSendTestEmail(permissions)
  const selectedCategory: TemplateCategory | 'all' = search.category ?? 'all'

  // Search: the box filters at once; the URL follows 300 ms after the last keystroke, in place.
  const searchQ = search.q ?? ''
  const [searchInput, setSearchInput] = useState(searchQ)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  // The last value written to the URL by the debounce, so its echo does not reset the box.
  const lastNavigatedQRef = useRef<string | undefined>(undefined)

  const handleSearchChange = (value: string) => {
    setSearchInput(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      const trimmed = value.trim()
      lastNavigatedQRef.current = trimmed
      navigate({
        replace: true,
        search: (prev: TemplatesSearch) => ({ ...prev, q: trimmed || undefined })
      })
    }, 300)
  }

  // Back/forward and links change the box; our own debounced writes do not.
  useEffect(() => {
    if (searchQ !== lastNavigatedQRef.current) {
      if (debounceRef.current) clearTimeout(debounceRef.current)
      setSearchInput(searchQ)
      lastNavigatedQRef.current = searchQ
    }
  }, [searchQ])

  useEffect(
    () => () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    },
    []
  )

  const setCategory = (category: TemplateCategory | 'all') =>
    navigate({
      search: (prev: TemplatesSearch) => ({
        ...prev,
        category: category === 'all' ? undefined : category
      })
    })

  const setPreview = (id?: string) =>
    navigate({
      replace: true,
      search: (prev: TemplatesSearch) => ({ ...prev, preview: id })
    })

  // T1: every email template at once; the category is filtered here so each tab has its count.
  const listQuery = useQuery({
    queryKey: ['templates', workspaceId, 'email'],
    queryFn: () => templatesApi.list({ workspace_id: workspaceId, channel: 'email' })
  })
  const allTemplates = useMemo(() => listQuery.data?.templates ?? [], [listQuery.data])

  // T2: what uses each template, read again on each visit of the page.
  const usageQuery = useQuery({
    queryKey: ['templates-usage', workspaceId],
    queryFn: () => templatesApi.usage({ workspace_id: workspaceId }),
    refetchOnMount: 'always'
  })

  const counts = useMemo(() => {
    const byCategory = new Map<string, number>()
    for (const template of allTemplates) {
      byCategory.set(template.category, (byCategory.get(template.category) ?? 0) + 1)
    }
    return byCategory
  }, [allTemplates])

  const tabs: CountTabOption<TemplateCategory | 'all'>[] = [
    { value: 'all', label: t`All`, count: listQuery.isSuccess ? allTemplates.length : undefined },
    ...TEMPLATE_CATEGORIES.filter(
      (category) => (counts.get(category) ?? 0) > 0 || category === selectedCategory
    ).map((category) => ({
      value: category,
      label: i18n._(TEMPLATE_CATEGORY_LABELS[category]),
      count: listQuery.isSuccess ? (counts.get(category) ?? 0) : undefined
    }))
  ]

  const inCategory =
    selectedCategory === 'all'
      ? allTemplates
      : allTemplates.filter((template) => template.category === selectedCategory)
  const query = searchInput.trim().toLowerCase()
  const visible = query
    ? inCategory.filter(
        (template) =>
          template.name.toLowerCase().includes(query) ||
          (template.email?.subject ?? '').toLowerCase().includes(query)
      )
    : inCategory

  // Preview in the URL: from the list, or read on its own when the list does not hold it.
  const previewId = search.preview
  const listedPreview = previewId ? allTemplates.find((tpl) => tpl.id === previewId) : undefined
  const previewQuery = useQuery({
    queryKey: ['template', workspaceId, previewId],
    queryFn: () => templatesApi.get({ workspace_id: workspaceId, id: previewId as string }),
    enabled: Boolean(previewId) && listQuery.isSuccess && !listedPreview
  })
  const fetchedPreview = previewQuery.data?.template
  // A web template (blog) is not on this page and has no email to preview.
  const fetchedEmail =
    fetchedPreview && !fetchedPreview.deleted_at && fetchedPreview.channel === 'email'
      ? fetchedPreview
      : undefined
  const previewTemplate = listedPreview ?? fetchedEmail
  const lookedUp = Boolean(previewId) && listQuery.isSuccess && !listedPreview
  // Only a 404 or a deleted template means the link is dead; another error keeps it, for a reload.
  const previewMissing =
    lookedUp &&
    ((previewQuery.error instanceof ApiError && previewQuery.error.status === 404) ||
      Boolean(fetchedPreview?.deleted_at))
  const previewElsewhere =
    lookedUp && Boolean(fetchedPreview) && !fetchedPreview?.deleted_at && !fetchedEmail
  const previewFailed = lookedUp && previewQuery.isError && !previewMissing

  useEffect(() => {
    if (!previewMissing && !previewElsewhere) return
    if (previewMissing) message.info(t`This template no longer exists`)
    setPreview(undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- setPreview only navigates
  }, [previewMissing, previewElsewhere])

  useEffect(() => {
    if (previewFailed) message.error(t`Couldn't load this template`)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- message is stable
  }, [previewFailed])

  const [drawer, setDrawer] = useState<DrawerState | null>(null)
  const drawerKey = useRef(0)
  const openDrawer = (opening: DrawerOpening) => {
    drawerKey.current += 1
    setDrawer({ ...opening, key: drawerKey.current })
  }

  const [testTemplate, setTestTemplate] = useState<Template | null>(null)
  const [toDelete, setToDelete] = useState<Template | null>(null)

  const deleteMutation = useMutation({
    mutationFn: (template: Template) =>
      templatesApi.delete({ workspace_id: workspaceId, id: template.id }),
    onSuccess: () => {
      message.success(t`Template deleted successfully`)
      setToDelete(null)
      void queryClient.invalidateQueries({ queryKey: ['templates', workspaceId] })
      void queryClient.invalidateQueries({ queryKey: ['templates-usage', workspaceId] })
    },
    onError: (error: Error & { response?: { data?: { error?: string } } }) => {
      const errorMsg = error?.response?.data?.error || error.message
      message.error(t`Failed to delete template: ${errorMsg}`)
    }
  })

  // Import: the same reading and checks as the editor's button, then a new template to save.
  const jsonInput = useRef<HTMLInputElement>(null)
  const mjmlInput = useRef<HTMLInputElement>(null)
  const [importErrors, setImportErrors] = useState<{ title: string; errors: string[] } | null>(null)
  const handleImport = async (event: ChangeEvent<HTMLInputElement>, kind: TemplateFileKind) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    const result = await importTemplateFile(file, kind, i18n)
    if (!result.ok) {
      if (result.title) setImportErrors({ title: result.title, errors: result.errors })
      else message.error(result.errors[0])
      return
    }
    openDrawer({
      mode: 'import',
      name: templateNameFromFile(file.name),
      tree: result.tree,
      testData: result.testData
    })
    for (const warning of result.warnings) {
      modal.warning({ title: warning.title, content: warning.content })
    }
  }

  const noWriteTooltip = canWrite ? undefined : t`You don't have write permission for templates`

  const marketingProvider = workspace?.integrations?.find(
    (integration) => integration.id === workspace.settings.marketing_email_provider_id
  )
  const transactionalProvider = workspace?.integrations?.find(
    (integration) => integration.id === workspace.settings.transactional_email_provider_id
  )

  const timezone = workspace?.settings.timezone
  const fullDate = (date: string) => dayjs(date).tz(timezone).format('llll')

  const hiddenUsage = (usageQuery.data?.hidden_types?.length ?? 0) > 0

  // A write the member cannot make says why, as the header buttons do.
  const withNoWriteTip = (label: string) =>
    noWriteTooltip ? (
      <Tooltip title={noWriteTooltip}>
        <span>{label}</span>
      </Tooltip>
    ) : (
      label
    )

  const menuItems = (template: Template): MenuProps['items'] => [
    {
      key: 'edit',
      label: withNoWriteTip(t`Edit`),
      disabled: !canWrite,
      onClick: () => openDrawer({ mode: 'edit', template })
    },
    { key: 'preview', label: t`Preview`, onClick: () => setPreview(template.id) },
    {
      key: 'test',
      label: t`Send test`,
      disabled: !canTest,
      onClick: () => setTestTemplate(template)
    },
    {
      key: 'clone',
      label: withNoWriteTip(t`Clone`),
      disabled: !canWrite,
      onClick: () => openDrawer({ mode: 'clone', template })
    },
    {
      key: 'copy',
      label: t`Copy ID`,
      onClick: async () => {
        if (await copyToClipboard(template.id)) message.success(t`ID copied`)
      }
    },
    { type: 'divider' },
    {
      key: 'delete',
      label: template.integration_id ? (
        <Tooltip title={t`This template is managed by an integration and cannot be deleted`}>
          <span>{t`Delete`}</span>
        </Tooltip>
      ) : (
        withNoWriteTip(t`Delete`)
      ),
      danger: true,
      disabled: !canWrite || Boolean(template.integration_id),
      onClick: () => setToDelete(template)
    }
  ]

  const renderUsage = (template: Template) => {
    if (usageQuery.isPending) {
      return <Skeleton.Input active size="small" />
    }
    if (usageQuery.isError) {
      return (
        <Tooltip title={t`Couldn't load`}>
          <span className="text-ink-4">—</span>
        </Tooltip>
      )
    }
    const summary = summarizeUsage(usageQuery.data.usage?.[template.id])
    if (!summary.label) return <span className="text-ink-4">{t`Not used`}</span>
    return summary.details.length > 1 ? (
      <Tooltip
        title={
          <div className="flex flex-col">
            {summary.details.map((line) => (
              <span key={line}>{line}</span>
            ))}
          </div>
        }
      >
        <span className="cursor-help text-ink-2">{summary.label}</span>
      </Tooltip>
    ) : (
      <span className="text-ink-2">{summary.label}</span>
    )
  }

  const columns: TableColumnType<Template>[] = [
    {
      title: t`Template`,
      key: 'name',
      render: (_: unknown, template: Template) => {
        const integration = workspace?.integrations?.find(
          (item) => item.id === template.integration_id
        )
        const subject = template.email?.subject ?? ''
        const preview = template.email?.subject_preview
        const subjectLine = preview ? `${subject} · ${preview}` : subject
        return (
          <div className="flex min-w-0 items-start gap-2">
            {template.integration_id && integration ? (
              <Tooltip title={t`Managed by ${integration.name} (${integration.type} integration)`}>
                <span className="mt-0.5 inline-flex shrink-0">
                  {integrationIcon(integration.type)}
                </span>
              </Tooltip>
            ) : null}
            <div className="flex min-w-0 flex-col">
              <div className="flex items-center gap-1.5">
                <Tooltip title={t`ID for API: ${template.id}`}>
                  <span className="font-medium text-ink">{template.name}</span>
                </Tooltip>
                {template.email?.editor_mode === 'code' ? (
                  <Pill tone="neutral" dot={false}>
                    {t`Code`}
                  </Pill>
                ) : null}
              </div>
              {subjectLine ? (
                <Tooltip title={subjectLine}>
                  <div className="max-w-[420px] truncate text-xs text-ink-3">
                    {subject}
                    {preview ? <span className="text-ink-4"> · {preview}</span> : null}
                  </div>
                </Tooltip>
              ) : null}
            </div>
          </div>
        )
      }
    },
    {
      title: t`Category`,
      key: 'category',
      render: (_: unknown, template: Template) => (
        <Pill tone={template.category === 'marketing' ? 'primary' : 'neutral'} dot={false}>
          {isTemplateCategory(template.category)
            ? i18n._(TEMPLATE_CATEGORY_LABELS[template.category])
            : template.category}
        </Pill>
      )
    },
    {
      title: (
        <Tooltip
          title={
            <div className="flex flex-col gap-1">
              <span>{t`Newsletters that may still send, transactional notifications, automations and double opt-in lists. Segment filters on a template aren't counted.`}</span>
              {hiddenUsage ? <span>{t`Uses you can't see aren't counted.`}</span> : null}
            </div>
          }
        >
          <span className="cursor-help">{t`Used in`}</span>
        </Tooltip>
      ),
      key: 'usage',
      render: (_: unknown, template: Template) => renderUsage(template)
    },
    {
      title: t`Sender`,
      key: 'sender',
      render: (_: unknown, template: Template) => {
        if (template.email?.sender_id) {
          const provider =
            template.category === 'marketing' ? marketingProvider : transactionalProvider
          const sender = provider?.email_provider?.senders.find(
            (item) => item.id === template.email?.sender_id
          )
          if (sender) return `${sender.name} <${sender.email}>`
        }
        return <span className="text-ink-3">{t`default`}</span>
      }
    },
    {
      title: t`Updated`,
      key: 'updated',
      render: (_: unknown, template: Template) => (
        <Tooltip
          title={t`Created ${fullDate(template.created_at)} · Updated ${fullDate(template.updated_at)} in ${timezone ?? 'UTC'}`}
        >
          <span className="whitespace-nowrap text-ink-3">
            v{template.version} · {relative(template.updated_at)}
          </span>
        </Tooltip>
      )
    },
    {
      key: 'actions',
      width: 48,
      render: (_: unknown, template: Template) => (
        <div onClick={(event) => event.stopPropagation()}>
          <Dropdown
            trigger={['click']}
            placement="bottomRight"
            menu={{ items: menuItems(template) }}
          >
            <Button
              type="text"
              size="small"
              aria-label={t`More actions`}
              icon={<MoreHorizontal size={16} aria-hidden="true" />}
            />
          </Dropdown>
        </div>
      )
    }
  ]

  const renderBody = () => {
    if (listQuery.isPending) {
      return (
        <div data-testid="templates-skeleton" className="px-3.5 py-3">
          <Skeleton active title={false} paragraph={{ rows: 6 }} />
        </div>
      )
    }
    if (listQuery.isError) {
      return (
        <Result
          status="error"
          title={t`Couldn't load the templates`}
          subTitle={listQuery.error.message}
          extra={<Button onClick={() => void listQuery.refetch()}>{t`Retry`}</Button>}
        />
      )
    }
    if (allTemplates.length === 0) {
      return (
        <div className="py-10">
          <Empty
            description={
              <div className="flex flex-col gap-1">
                <span className="font-medium text-ink-2">{t`No templates found`}</span>
                <span>{t`Create your first template to get started`}</span>
              </div>
            }
          >
            <Tooltip title={noWriteTooltip}>
              <span className="inline-block">
                <Button
                  type="primary"
                  icon={<Plus size={14} aria-hidden="true" />}
                  disabled={!canWrite}
                  onClick={() => openDrawer({ mode: 'create' })}
                >
                  {t`New template`}
                </Button>
              </span>
            </Tooltip>
          </Empty>
        </div>
      )
    }
    if (inCategory.length === 0 && selectedCategory !== 'all') {
      const label = i18n._(TEMPLATE_CATEGORY_LABELS[selectedCategory])
      return (
        <div className="py-10">
          <Empty
            description={
              <div className="flex flex-col gap-1">
                <span className="font-medium text-ink-2">
                  {t`No templates found for category "${label}"`}
                </span>
                <span>
                  {t`Try selecting a different category or`}{' '}
                  <Button type="link" size="small" onClick={() => setCategory('all')}>
                    {t`reset the filter`}
                  </Button>
                </span>
              </div>
            }
          />
        </div>
      )
    }
    return (
      <Table<Template>
        columns={columns}
        dataSource={visible}
        rowKey="id"
        pagination={false}
        locale={{ emptyText: t`No templates match your search` }}
        onRow={(template) => ({
          style: { cursor: 'pointer' },
          onClick: () =>
            canWrite ? openDrawer({ mode: 'edit', template }) : setPreview(template.id)
        })}
      />
    )
  }

  const importMenu: MenuProps['items'] = [
    { key: 'json', label: t`Notifuse export (.json)`, onClick: () => jsonInput.current?.click() },
    { key: 'mjml', label: t`MJML file (.mjml)`, onClick: () => mjmlInput.current?.click() }
  ]

  return (
    <div className="flex flex-col gap-3">
      <PageHeader
        title={t`Templates`}
        count={listQuery.isSuccess ? allTemplates.length : undefined}
        actions={
          <>
            <Tooltip title={noWriteTooltip}>
              <span className="inline-block">
                <Dropdown trigger={['click']} menu={{ items: importMenu }} disabled={!canWrite}>
                  <Button icon={<Upload size={14} aria-hidden="true" />} disabled={!canWrite}>
                    {t`Import`}
                  </Button>
                </Dropdown>
              </span>
            </Tooltip>
            <Tooltip title={noWriteTooltip}>
              <span className="inline-block">
                <Button
                  type="primary"
                  icon={<Plus size={14} aria-hidden="true" />}
                  disabled={!canWrite}
                  onClick={() => openDrawer({ mode: 'create' })}
                >
                  {t`New template`}
                </Button>
              </span>
            </Tooltip>
          </>
        }
      />

      <Panel flush>
        <TableToolbar
          start={
            <CountTabs<TemplateCategory | 'all'>
              options={tabs}
              value={selectedCategory}
              onChange={setCategory}
            />
          }
          end={
            <TableToolbar.Search
              value={searchInput}
              onChange={handleSearchChange}
              placeholder={t`Name or subject`}
            />
          }
        />
        {renderBody()}
      </Panel>

      <input
        ref={jsonInput}
        type="file"
        accept=".json,application/json"
        className="hidden"
        onChange={(event) => void handleImport(event, 'json')}
      />
      <input
        ref={mjmlInput}
        type="file"
        accept=".mjml,.xml,text/xml,application/xml,text/plain"
        className="hidden"
        onChange={(event) => void handleImport(event, 'mjml')}
      />

      {workspace && drawer ? (
        <CreateTemplateDrawer
          key={drawer.key}
          open
          onClose={() => setDrawer(null)}
          workspace={workspace}
          canManageSavedBlocks={canWrite}
          template={drawer.mode === 'edit' ? drawer.template : undefined}
          fromTemplate={drawer.mode === 'clone' ? drawer.template : undefined}
          initialName={drawer.mode === 'import' ? drawer.name : undefined}
          initialTree={drawer.mode === 'import' ? drawer.tree : undefined}
          initialTestData={drawer.mode === 'import' ? drawer.testData : undefined}
        />
      ) : null}

      {workspace && previewTemplate ? (
        <TemplatePreviewDrawer
          key={previewTemplate.id}
          record={previewTemplate}
          workspace={workspace}
          open
          onClose={() => setPreview(undefined)}
        />
      ) : null}

      <DeleteTemplateModal
        workspaceId={workspaceId}
        template={toDelete}
        deleting={deleteMutation.isPending}
        onConfirm={(template) => deleteMutation.mutate(template)}
        onCancel={() => setToDelete(null)}
      />

      <ImportErrorsModal
        open={Boolean(importErrors)}
        title={importErrors?.title ?? ''}
        errors={importErrors?.errors ?? []}
        onClose={() => setImportErrors(null)}
      />

      {workspace ? (
        <SendTemplateModal
          isOpen={Boolean(testTemplate)}
          onClose={() => setTestTemplate(null)}
          template={testTemplate}
          workspace={workspace}
          canReadLists={!!permissions?.lists?.read}
          canReadContacts={!!permissions?.contacts?.read}
        />
      ) : null}
    </div>
  )
}
