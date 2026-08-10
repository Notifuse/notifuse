import { useMemo, useState } from 'react'
import { Button, Checkbox, Empty, Modal, Tag } from 'antd'
import { CloseOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../context'
import {
  DimensionCategory,
  DimensionInfo,
  dimensionsForSchema,
  getDimensionLabel,
  groupByCategory
} from '../lib/dimensions'

interface BreakdownModalProps {
  open: boolean
  onCancel: () => void
  onSubmit: (dimensions: string[]) => void
  /** Dimensions already pinned by the parent row, so breaking down by them is a no-op. */
  excludeDimensions?: string[]
  initialDimensions?: string[]
  title?: string
  submitText?: string
}

const COLUMNS = 4

interface CategoryGroup {
  category: DimensionCategory
  dimensions: DimensionInfo[]
}

/**
 * Spreads the categories over the grid columns without reordering them, by
 * filling each column until it holds its share of the rows. Balancing by row
 * count rather than by category keeps a ten-slot category from leaving three
 * columns nearly empty.
 */
function toColumns(groups: CategoryGroup[]): CategoryGroup[][] {
  const total = groups.reduce((sum, group) => sum + group.dimensions.length, 0)
  const target = Math.ceil(total / COLUMNS)
  const columns: CategoryGroup[][] = Array.from({ length: COLUMNS }, () => [])

  let index = 0
  let filled = 0
  for (const group of groups) {
    if (filled >= target && index < COLUMNS - 1) {
      index += 1
      filled = 0
    }
    columns[index].push(group)
    filled += group.dimensions.length
  }
  return columns
}

/** Multi-select over the dimension catalog, used for breakdowns and for building a report from scratch. */
export function BreakdownModal(props: BreakdownModalProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const [selected, setSelected] = useState<string[]>(props.initialDimensions ?? [])

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

  const columns = useMemo(() => {
    const excluded = new Set(props.excludeDimensions ?? [])
    const available = dimensionsForSchema('web_sessions').filter(
      (entry) => !excluded.has(entry.name)
    )
    return toColumns(groupByCategory(available))
  }, [props.excludeDimensions])

  const toggle = (dimension: string) => {
    setSelected((current) =>
      current.includes(dimension)
        ? current.filter((candidate) => candidate !== dimension)
        : [...current, dimension]
    )
  }

  const isEmpty = columns.every((column) => column.length === 0)

  return (
    <Modal
      open={props.open}
      title={props.title ?? t`Breakdown by`}
      width={1000}
      centered
      onCancel={props.onCancel}
      // Reset on every open so a cancelled selection never leaks into the next one.
      afterOpenChange={(open) => {
        if (open) setSelected(props.initialDimensions ?? [])
      }}
      styles={{ body: { height: 560, display: 'flex', flexDirection: 'column' } }}
      footer={[
        <Button key="cancel" onClick={props.onCancel}>
          {t`Cancel`}
        </Button>,
        <Button
          key="submit"
          type="primary"
          disabled={selected.length === 0}
          onClick={() => props.onSubmit(selected)}
        >
          {props.submitText ?? t`View breakdown`}
        </Button>
      ]}
    >
      <div className="mb-4 flex min-h-[46px] flex-wrap items-center gap-2 rounded-lg bg-gray-50 p-3">
        <span className="text-xs font-medium text-gray-500">{t`Selected dimensions, in order:`}</span>
        {selected.length === 0 ? (
          <span className="text-xs italic text-gray-400">{t`Pick a dimension below`}</span>
        ) : (
          selected.map((dimension, index) => (
            <Tag
              key={dimension}
              color="blue"
              closable
              className="!m-0"
              closeIcon={<CloseOutlined className="text-[10px]" />}
              onClose={(event) => {
                event.preventDefault()
                toggle(dimension)
              }}
            >
              {index + 1}. {getDimensionLabel(dimension, context.customDimensionLabels)}
            </Tag>
          ))
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto rounded-lg border border-gray-200 p-3">
        {isEmpty ? (
          <div className="p-8">
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No dimensions found`} />
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            {columns.map((column, index) => (
              <div key={index}>
                {column.map((group) => (
                  <div key={group.category} className="mb-4">
                    <div className="mb-2 text-xs font-semibold uppercase text-[var(--primary)]">
                      {categoryLabels[group.category] ?? group.category}
                    </div>
                    <div className="space-y-1">
                      {group.dimensions.map((entry) => (
                        <button
                          key={entry.name}
                          type="button"
                          onClick={() => toggle(entry.name)}
                          className="flex w-full items-center gap-3 rounded py-1 text-left hover:bg-gray-50"
                        >
                          <Checkbox checked={selected.includes(entry.name)} />
                          <span className="text-sm">
                            {getDimensionLabel(entry.name, context.customDimensionLabels)}
                          </span>
                        </button>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            ))}
          </div>
        )}
      </div>
    </Modal>
  )
}
