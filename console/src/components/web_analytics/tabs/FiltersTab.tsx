import { useMemo, useState } from 'react'
import { App, Button, Empty, Input, Segmented, Space } from 'antd'
import { ExperimentOutlined, PlusOutlined, SearchOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { useAuth } from '../../../contexts/AuthContext'
import { WebFilter, webAnalyticsService } from '../../../services/api/web_analytics'
import { useWebAnalytics } from '../context'
import { BackfillStatus } from '../filters/BackfillStatus'
import { FilterDraft, FilterFormDrawer } from '../filters/FilterFormDrawer'
import { FilterTable } from '../filters/FilterTable'
import { TestFilterModal } from '../filters/TestFilterModal'

const ALL_TAGS = 'all'

export function FiltersTab() {
  const { t } = useLingui()
  const { message } = App.useApp()
  const { refreshWorkspaces } = useAuth()
  const { workspaceId, settings, customDimensionLabels, tag, setTag } = useWebAnalytics()

  const [searchText, setSearchText] = useState('')
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [editingFilter, setEditingFilter] = useState<WebFilter | undefined>()
  const [testModalOpen, setTestModalOpen] = useState(false)
  const [saving, setSaving] = useState(false)

  const rules = useMemo(() => settings?.filters ?? [], [settings])

  const tags = useMemo(() => {
    const collected = new Set<string>()
    for (const rule of rules) {
      for (const tag of rule.tags ?? []) collected.add(tag)
    }
    return Array.from(collected).sort((a, b) => a.localeCompare(b))
  }, [rules])

  // The selection lives in the URL so a narrowed rule list can be linked to.
  // A tag that no rule carries any more falls back to showing everything.
  const selectedTag = tag && tags.includes(tag) ? tag : ALL_TAGS

  const visibleRules = useMemo(() => {
    if (selectedTag === ALL_TAGS) return rules
    return rules.filter((rule) => (rule.tags ?? []).includes(selectedTag))
  }, [rules, selectedTag])

  /**
   * Notifuse keeps the whole rule set inside the workspace settings, so there
   * is no per-rule endpoint and no dirty state: every edit rewrites the array
   * and the server recomputes filters_version from it.
   */
  const persist = async (next: WebFilter[], successMessage: string): Promise<boolean> => {
    if (!settings) return false
    setSaving(true)
    try {
      await webAnalyticsService.setSettings(workspaceId, { ...settings, filters: next })
      await refreshWorkspaces()
      message.success(successMessage)
      return true
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error))
      return false
    } finally {
      setSaving(false)
    }
  }

  const handleSubmit = async (draft: FilterDraft) => {
    const now = new Date().toISOString()
    let next: WebFilter[]

    if (editingFilter) {
      next = rules.map((rule) =>
        rule.id === editingFilter.id ? { ...rule, ...draft, updated_at: now } : rule
      )
    } else {
      const maxOrder = rules.reduce((highest, rule) => Math.max(highest, rule.order), 0)
      next = [
        ...rules,
        { id: crypto.randomUUID(), order: maxOrder + 1, created_at: now, updated_at: now, ...draft }
      ]
    }

    const saved = await persist(next, editingFilter ? t`Rule updated` : t`Rule created`)
    if (saved) closeDrawer()
  }

  const handleDelete = (rule: WebFilter) => {
    void persist(
      rules.filter((candidate) => candidate.id !== rule.id),
      t`Rule deleted`
    )
  }

  const handleToggleEnabled = (rule: WebFilter) => {
    void persist(
      rules.map((candidate) =>
        candidate.id === rule.id
          ? { ...candidate, enabled: !candidate.enabled, updated_at: new Date().toISOString() }
          : candidate
      ),
      rule.enabled ? t`Rule disabled` : t`Rule enabled`
    )
  }

  const openCreate = () => {
    setEditingFilter(undefined)
    setDrawerOpen(true)
  }

  const openEdit = (rule: WebFilter) => {
    setEditingFilter(rule)
    setDrawerOpen(true)
  }

  const closeDrawer = () => {
    setDrawerOpen(false)
    setEditingFilter(undefined)
  }

  return (
    <div>
      <div className="mb-6">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-medium text-gray-800">{t`Attribution rules`}</h2>
          <Space>
            <Button type="text" icon={<ExperimentOutlined />} onClick={() => setTestModalOpen(true)}>
              {t`Test`}
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              {t`Add`}
            </Button>
          </Space>
        </div>
        <p className="mt-3 text-gray-500 md:mt-1">
          {t`Map traffic to channels, fill custom dimensions and rewrite traffic source fields as sessions come in.`}
        </p>
      </div>

      <BackfillStatus
        workspaceId={workspaceId}
        filtersVersion={settings?.filters_version}
        hasRules={rules.length > 0}
      />

      {rules.length > 0 ? (
        <div className="mb-4 flex flex-col gap-4 md:flex-row md:items-center">
          {tags.length > 0 ? (
            <Segmented
              value={selectedTag}
              onChange={(value) => setTag(value === ALL_TAGS ? undefined : String(value))}
              options={[
                { value: ALL_TAGS, label: t`All` },
                ...tags.map((tag) => ({ value: tag, label: tag }))
              ]}
            />
          ) : null}
          <div className="w-full md:ml-auto md:w-[300px]">
            <Input
              placeholder={t`Search...`}
              allowClear
              prefix={<SearchOutlined className="text-gray-400" />}
              value={searchText}
              onChange={(event) => setSearchText(event.target.value)}
            />
          </div>
        </div>
      ) : null}

      {visibleRules.length > 0 ? (
        <FilterTable
          filters={visibleRules}
          searchText={searchText}
          customDimensionLabels={customDimensionLabels}
          saving={saving}
          onEdit={openEdit}
          onDelete={handleDelete}
          onToggleEnabled={handleToggleEnabled}
        />
      ) : rules.length > 0 ? (
        <Empty description={t`No rule carries the selected tag`} className="py-12" />
      ) : (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={
            <span className="text-gray-500">
              {t`No attribution rules yet. Create the first one to start shaping your traffic data.`}
            </span>
          }
          className="py-12"
        >
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            {t`Create rule`}
          </Button>
        </Empty>
      )}

      <FilterFormDrawer
        open={drawerOpen}
        filter={editingFilter}
        existingTags={tags}
        customDimensionLabels={customDimensionLabels}
        saving={saving}
        onClose={closeDrawer}
        onSubmit={handleSubmit}
      />

      <TestFilterModal
        open={testModalOpen}
        onClose={() => setTestModalOpen(false)}
        filters={rules}
        customDimensionLabels={customDimensionLabels}
      />
    </div>
  )
}
