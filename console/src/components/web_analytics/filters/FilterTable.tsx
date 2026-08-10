import { MouseEvent, useMemo, useState } from 'react'
import { Button, Popconfirm, Popover, Space, Switch, Table, Tag, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { DeleteOutlined, EditOutlined } from '@ant-design/icons'
import { ChevronDown } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { WebFilter, WebFilterCondition } from '../../../services/api/web_analytics'
import { getDimensionLabel } from '../lib/dimensions'
import { getSourceFieldLabel, useFilterVocabulary } from '../lib/filterCatalog'

export interface FilterTableProps {
  filters: WebFilter[]
  searchText?: string
  customDimensionLabels?: Record<string, string>
  /** Every mutation rewrites the whole rule array, so one flag covers them all. */
  saving?: boolean
  onEdit: (filter: WebFilter) => void
  onDelete: (filter: WebFilter) => void
  onToggleEnabled: (filter: WebFilter) => void
}

function ConditionTags(props: { conditions: WebFilterCondition[] }) {
  const vocabulary = useFilterVocabulary()
  return (
    <>
      {props.conditions.map((condition, index) => (
        <div key={index} className="inline-flex flex-wrap items-center gap-1 text-sm">
          <Tag variant="filled" color="green">
            {getSourceFieldLabel(condition.field)}
          </Tag>
          <Tag variant="filled" color="blue">
            {vocabulary.operators[condition.operator]}
          </Tag>
          {condition.value ? <Tag>{condition.value}</Tag> : null}
        </div>
      ))}
    </>
  )
}

interface MobileFilterCardProps {
  filter: WebFilter
  isExpanded: boolean
  disabled?: boolean
  customDimensionLabels?: Record<string, string>
  onToggle: () => void
  onEdit: () => void
  onDelete: () => void
  onToggleEnabled: () => void
}

function MobileFilterCard(props: MobileFilterCardProps) {
  const { t } = useLingui()
  const vocabulary = useFilterVocabulary()
  const tags = props.filter.tags ?? []

  return (
    <div className="mb-3 rounded-lg border border-gray-200 bg-white p-4">
      <div className="flex cursor-pointer items-center justify-between" onClick={props.onToggle}>
        <div className="min-w-0 flex-1">
          <div className={`truncate font-medium ${props.filter.enabled ? '' : 'text-gray-400'}`}>
            {props.filter.name}
          </div>
          {tags.length > 0 ? (
            <div className="mt-1 flex flex-wrap gap-1">
              {tags.map((tag) => (
                <Tag key={tag} className="text-xs">
                  {tag}
                </Tag>
              ))}
            </div>
          ) : null}
        </div>
        <div className="flex items-center gap-2">
          <Switch
            checked={props.filter.enabled}
            size="small"
            disabled={props.disabled}
            onClick={(_, event) => {
              event.stopPropagation()
              props.onToggleEnabled()
            }}
          />
          <ChevronDown
            size={16}
            className={`text-gray-400 transition-transform ${props.isExpanded ? 'rotate-180' : ''}`}
          />
        </div>
      </div>

      {props.isExpanded ? (
        <div className="mt-4 space-y-4 border-t border-gray-100 pt-4">
          <div className="text-sm">
            <span className="text-gray-500">{t`Priority:`}</span>
            <span className="ml-1 font-medium">{props.filter.priority}</span>
          </div>

          <div>
            <div className="mb-2 text-xs font-medium uppercase text-gray-400">{t`Conditions`}</div>
            {props.filter.conditions.length === 0 ? (
              <span className="text-sm italic text-gray-400">{t`(always matches)`}</span>
            ) : (
              <div className="space-y-2">
                <ConditionTags conditions={props.filter.conditions} />
              </div>
            )}
          </div>

          <div>
            <div className="mb-2 text-xs font-medium uppercase text-gray-400">{t`Operations`}</div>
            <div className="space-y-2">
              {props.filter.operations.map((operation, index) => (
                <div key={index} className="flex flex-wrap items-center gap-1 text-sm">
                  <Tag variant="filled" color="purple">
                    {getDimensionLabel(operation.dimension, props.customDimensionLabels)}
                  </Tag>
                  <Tag variant="filled" color="orange">
                    {vocabulary.actionShorthands[operation.action]}
                  </Tag>
                  {operation.action !== 'unset_value' && operation.value ? (
                    <Tag>{operation.value}</Tag>
                  ) : null}
                </div>
              ))}
            </div>
          </div>

          <div className="flex gap-2 pt-2">
            <Popconfirm
              title={t`Delete this rule?`}
              onConfirm={props.onDelete}
              okText={t`Delete`}
              cancelText={t`Cancel`}
            >
              <Button block size="small" icon={<DeleteOutlined />} disabled={props.disabled}>
                {t`Delete`}
              </Button>
            </Popconfirm>
            <Button block size="small" icon={<EditOutlined />} onClick={props.onEdit}>
              {t`Edit`}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  )
}

export function FilterTable(props: FilterTableProps) {
  const { t } = useLingui()
  const vocabulary = useFilterVocabulary()
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())

  const { filters, searchText = '', customDimensionLabels } = props

  const toggleExpanded = (id: string) => {
    setExpandedIds((previous) => {
      const next = new Set(previous)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const displayFilters = useMemo(() => {
    let result = [...filters].sort((a, b) => b.priority - a.priority)
    if (searchText) {
      const needle = searchText.toLowerCase()
      result = result.filter(
        (filter) =>
          filter.name.toLowerCase().includes(needle) ||
          filter.conditions.some(
            (condition) =>
              condition.field.toLowerCase().includes(needle) ||
              (condition.value?.toLowerCase().includes(needle) ?? false)
          ) ||
          filter.operations.some((operation) => operation.dimension.toLowerCase().includes(needle))
      )
    }
    return result
  }, [filters, searchText])

  const columns: ColumnsType<WebFilter> = [
    {
      title: t`Priority`,
      dataIndex: 'priority',
      key: 'priority',
      width: 80,
      render: (priority: number) => <span className="text-gray-600">{priority}</span>
    },
    {
      title: t`Name`,
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record) => (
        <span className={record.enabled ? 'font-medium' : 'text-gray-400'}>{name}</span>
      )
    },
    {
      title: t`Conditions`,
      key: 'conditions',
      render: (_, record) => {
        if (record.conditions.length === 0) {
          return <span className="italic text-gray-400">{t`(always matches)`}</span>
        }
        const visible = record.conditions.slice(0, 2)
        const hidden = record.conditions.slice(2)
        return (
          <div className="flex flex-col gap-1">
            <ConditionTags conditions={visible} />
            {hidden.length > 0 ? (
              <Popover
                content={
                  <div className="flex flex-col gap-1">
                    <ConditionTags conditions={hidden} />
                  </div>
                }
              >
                <span className="cursor-pointer text-xs text-gray-400 hover:text-gray-600">
                  {t`+${hidden.length} more`}
                </span>
              </Popover>
            ) : null}
          </div>
        )
      }
    },
    {
      title: t`Operations`,
      key: 'operations',
      render: (_, record) => (
        <div className="flex flex-col gap-1">
          {record.operations.map((operation, index) => (
            <div key={index} className="inline-flex flex-wrap items-center gap-1 text-sm">
              <Tooltip title={operation.dimension}>
                <Tag variant="filled" color="purple">
                  {getDimensionLabel(operation.dimension, customDimensionLabels)}
                </Tag>
              </Tooltip>
              <Tag variant="filled" color="orange">
                {vocabulary.actionShorthands[operation.action]}
              </Tag>
              {operation.action !== 'unset_value' && operation.value ? (
                <Tag>{operation.value}</Tag>
              ) : null}
            </div>
          ))}
        </div>
      )
    },
    {
      title: t`Tags`,
      dataIndex: 'tags',
      key: 'tags',
      render: (tags: string[] | undefined) => (
        <div className="flex flex-wrap gap-1">
          {(tags ?? []).map((tag) => (
            <Tag key={tag}>{tag}</Tag>
          ))}
        </div>
      )
    },
    {
      title: t`Status`,
      key: 'status',
      width: 80,
      render: (_, record) => (
        <Popconfirm
          title={record.enabled ? t`Disable this rule?` : t`Enable this rule?`}
          onConfirm={() => props.onToggleEnabled(record)}
          okText={t`Yes`}
          cancelText={t`No`}
        >
          <Switch checked={record.enabled} size="small" disabled={props.saving} />
        </Popconfirm>
      )
    },
    {
      title: '',
      key: 'actions',
      width: 80,
      className: 'actions-cell',
      render: (_, record) => (
        <Space
          size="small"
          className="opacity-0 transition-opacity [.ant-table-row:hover_&]:opacity-100"
        >
          <Popconfirm
            title={t`Delete this rule?`}
            onConfirm={() => props.onDelete(record)}
            okText={t`Delete`}
            cancelText={t`Cancel`}
          >
            <Button type="text" size="small" icon={<DeleteOutlined />} disabled={props.saving} />
          </Popconfirm>
          <Button
            type="text"
            size="small"
            icon={<EditOutlined />}
            onClick={() => props.onEdit(record)}
          />
        </Space>
      )
    }
  ]

  // The whole row opens the editor, except where the click was aimed at one of
  // the controls sitting inside it.
  const handleRowClick = (record: WebFilter, event: MouseEvent<HTMLElement>) => {
    const target = event.target as HTMLElement
    if (target.closest('.ant-switch, .ant-popover, .ant-btn, button, .ant-tag')) return
    props.onEdit(record)
  }

  return (
    <>
      <div className="md:hidden">
        {displayFilters.map((filter) => (
          <MobileFilterCard
            key={filter.id}
            filter={filter}
            isExpanded={expandedIds.has(filter.id)}
            disabled={props.saving}
            customDimensionLabels={customDimensionLabels}
            onToggle={() => toggleExpanded(filter.id)}
            onEdit={() => props.onEdit(filter)}
            onDelete={() => props.onDelete(filter)}
            onToggleEnabled={() => props.onToggleEnabled(filter)}
          />
        ))}
      </div>

      <div className="hidden md:block">
        <Table
          dataSource={displayFilters}
          columns={columns}
          rowKey="id"
          pagination={false}
          size="small"
          loading={props.saving}
          onRow={(record) => ({
            onClick: (event) => handleRowClick(record, event),
            className: 'cursor-pointer'
          })}
          className="border border-gray-200 rounded-md"
        />
      </div>
    </>
  )
}
