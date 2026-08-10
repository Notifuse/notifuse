import { useMemo, useState } from 'react'
import { Button, Dropdown, Empty, Input, Tooltip } from 'antd'
import { CloseOutlined, PlusCircleOutlined, SearchOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import {
  DndContext,
  DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors
} from '@dnd-kit/core'
import {
  SortableContext,
  arrayMove,
  horizontalListSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
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

interface SortableDimensionChipProps {
  dimension: string
  label: string
  isLast: boolean
  onRemove: () => void
}

/**
 * One level of the drill-down. Dragging it is what changes which level it is,
 * so the whole chip is the handle rather than a separate grip.
 */
function SortableDimensionChip(props: SortableDimensionChipProps) {
  const { t } = useLingui()
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: props.dimension
  })

  return (
    <span
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        // The chip stays in the row while dragging so the gap it leaves shows
        // where dropping it now would put it.
        opacity: isDragging ? 0.5 : 1
      }}
      className="inline-flex items-center"
    >
      <span
        {...attributes}
        {...listeners}
        className="inline-flex cursor-grab items-center gap-1 rounded border border-blue-200 bg-blue-50 py-0.5 pl-2 pr-1 text-xs text-blue-700 active:cursor-grabbing"
      >
        <span>{props.label}</span>
        <button
          type="button"
          aria-label={t`Remove dimension`}
          title={t`Remove dimension`}
          onClick={props.onRemove}
          className="text-blue-400 hover:text-blue-700"
        >
          <CloseOutlined className="text-[10px]" />
        </button>
      </span>
      {props.isLast ? null : <span className="mx-1 text-gray-400">›</span>}
    </span>
  )
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

  const sensors = useSensors(
    // Without a threshold, pressing the remove button would begin a drag
    // instead of clicking it.
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    // Lets the order be changed from the keyboard, which the chips used to
    // offer as a pair of nudge arrows.
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  )

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const from = props.value.indexOf(String(active.id))
    const to = props.value.indexOf(String(over.id))
    if (from < 0 || to < 0) return
    props.onChange(arrayMove(props.value, from, to))
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
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={props.value} strategy={horizontalListSortingStrategy}>
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
            <SortableDimensionChip
              key={dimension}
              dimension={dimension}
              label={getDimensionLabel(dimension, context.customDimensionLabels)}
              isLast={index === props.value.length - 1}
              onRemove={() => remove(dimension)}
            />
          ))}
        </div>
      </SortableContext>
    </DndContext>
  )
}
