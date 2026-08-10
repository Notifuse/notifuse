import { useMemo, useState } from 'react'
import { Button, Dropdown, Empty, Input, Tooltip } from 'antd'
import { CloseOutlined, PlusCircleOutlined, SearchOutlined } from '@ant-design/icons'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../context'
import {
  DIMENSION_EXAMPLES,
  dimensionsForSchema,
  getDimensionLabel,
  groupByCategory
} from '../lib/dimensions'

interface DimensionSelectorProps {
  value: string[]
  onChange: (dimensions: string[]) => void
}

/**
 * Picks the dimensions the report drills through, in order: the first chip is
 * the top level of the table, each following one a level below it. Order is
 * therefore part of the report, which is why the chips can be moved.
 */
export function DimensionSelector(props: DimensionSelectorProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)

  const categoryLabels: Record<string, string> = {
    Channel: t`Channel`,
    UTM: t`UTM`,
    Traffic: t`Traffic`,
    Pages: t`Pages`,
    Device: t`Device`,
    Geo: t`Geo`,
    Time: t`Time`,
    Session: t`Session`,
    Goal: t`Goal`,
    User: t`User`,
    Custom: t`Custom dimensions`
  }

  const available = useMemo(
    () => dimensionsForSchema('web_sessions').filter((entry) => !props.value.includes(entry.name)),
    [props.value]
  )

  const groups = useMemo(() => {
    const term = search.trim().toLowerCase()
    if (!term) return groupByCategory(available)
    return groupByCategory(
      available.filter(
        (entry) =>
          getDimensionLabel(entry.name, context.customDimensionLabels)
            .toLowerCase()
            .includes(term) || entry.category.toLowerCase().includes(term)
      )
    )
  }, [available, search, context.customDimensionLabels])

  const allSelected = available.length === 0

  const add = (dimension: string) => {
    if (!props.value.includes(dimension)) props.onChange([...props.value, dimension])
    setOpen(false)
    setSearch('')
  }

  const remove = (dimension: string) => {
    props.onChange(props.value.filter((candidate) => candidate !== dimension))
  }

  const move = (index: number, offset: number) => {
    const target = index + offset
    if (target < 0 || target >= props.value.length) return
    const next = [...props.value]
    const [moved] = next.splice(index, 1)
    next.splice(target, 0, moved)
    props.onChange(next)
  }

  const picker = (
    <div className="w-64 rounded-lg border border-gray-200 bg-white shadow-lg">
      <div className="border-b border-gray-100 p-2">
        <Input
          prefix={<SearchOutlined className="text-gray-400" />}
          allowClear
          placeholder={t`Search dimensions`}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </div>
      <div className="max-h-80 overflow-y-auto py-1">
        {groups.length === 0 ? (
          <div className="p-4">
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={allSelected ? t`All dimensions selected` : t`No dimensions found`}
            />
          </div>
        ) : (
          groups.map((group) => (
            <div key={group.category}>
              <div className="px-3 py-1 text-[10px] font-semibold uppercase text-[var(--primary)]">
                {categoryLabels[group.category] ?? group.category}
              </div>
              {group.dimensions.map((entry) => {
                const examples = DIMENSION_EXAMPLES[entry.name]
                return (
                  <Tooltip
                    key={entry.name}
                    placement="right"
                    title={
                      <div className="text-xs">
                        <div className="font-mono">{entry.name}</div>
                        {examples ? (
                          <div className="mt-1 opacity-70">{t`e.g. ${examples.join(', ')}`}</div>
                        ) : null}
                      </div>
                    }
                  >
                    <button
                      type="button"
                      onClick={() => add(entry.name)}
                      className="block w-full px-3 py-1.5 text-left text-sm hover:bg-gray-100"
                    >
                      {getDimensionLabel(entry.name, context.customDimensionLabels)}
                    </button>
                  </Tooltip>
                )
              })}
            </div>
          ))
        )}
      </div>
    </div>
  )

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Dropdown
        trigger={['click']}
        open={open}
        disabled={allSelected}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) setSearch('')
        }}
        popupRender={() => picker}
      >
        <Button type="link" size="small" icon={<PlusCircleOutlined />} disabled={allSelected}>
          {t`Add dimension`}
        </Button>
      </Dropdown>

      {props.value.map((dimension, index) => (
        <span key={dimension} className="inline-flex items-center">
          <span className="inline-flex items-center gap-1 rounded border border-blue-200 bg-blue-50 py-0.5 pl-1 pr-1.5 text-xs text-blue-700">
            {/* Drag and drop would need a dependency this console does not
                carry, so the level of a dimension is nudged one step at a time. */}
            <button
              type="button"
              aria-label={t`Move earlier`}
              title={t`Move earlier`}
              disabled={index === 0}
              onClick={() => move(index, -1)}
              className="text-blue-400 hover:text-blue-700 disabled:cursor-default disabled:opacity-30"
            >
              <ChevronLeft size={12} />
            </button>
            <span>{getDimensionLabel(dimension, context.customDimensionLabels)}</span>
            <button
              type="button"
              aria-label={t`Move later`}
              title={t`Move later`}
              disabled={index === props.value.length - 1}
              onClick={() => move(index, 1)}
              className="text-blue-400 hover:text-blue-700 disabled:cursor-default disabled:opacity-30"
            >
              <ChevronRight size={12} />
            </button>
            <button
              type="button"
              aria-label={t`Remove dimension`}
              title={t`Remove dimension`}
              onClick={() => remove(dimension)}
              className="ml-0.5 text-blue-400 hover:text-blue-700"
            >
              <CloseOutlined className="text-[10px]" />
            </button>
          </span>
          {index < props.value.length - 1 ? (
            <span className="mx-1 text-gray-400">›</span>
          ) : null}
        </span>
      ))}
    </div>
  )
}
