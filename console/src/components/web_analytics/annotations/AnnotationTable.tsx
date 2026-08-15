import { Button, Empty, Popconfirm, Table, Tag } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { DeleteOutlined, EditOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import { Annotation } from '../../../services/api/annotation'
import { ANNOTATION_DEFAULT_COLOR } from './AnnotationFormModal'

export interface AnnotationTableProps {
  annotations: Annotation[]
  loading?: boolean
  onEdit: (annotation: Annotation) => void
  onDelete: (annotation: Annotation) => void
}

/**
 * Renders the moment in the timezone it was entered in, which is the whole
 * point of storing one: the instant is already fixed by annotated_at.
 */
function formatMoment(annotation: Annotation): string {
  return dayjs(annotation.annotated_at).tz(annotation.timezone).format('MMM D, YYYY HH:mm')
}

function useDeletePrompt() {
  const { t } = useLingui()
  return (annotation: Annotation) =>
    annotation.source === 'broadcast'
      ? t`This annotation was created automatically when a broadcast started. Deleting it will not affect the broadcast.`
      : t`This action cannot be undone.`
}

export function AnnotationTable(props: AnnotationTableProps) {
  const { t } = useLingui()
  const { annotations, loading, onEdit, onDelete } = props
  const deletePrompt = useDeletePrompt()

  const columns: ColumnsType<Annotation> = [
    {
      title: t`When`,
      key: 'when',
      width: 220,
      render: (_, record) => (
        <div>
          <div className="font-semibold">{formatMoment(record)}</div>
          <div className="text-xs text-gray-400">{record.timezone}</div>
        </div>
      )
    },
    {
      title: t`Annotation`,
      key: 'annotation',
      render: (_, record) => (
        <div className="flex items-start gap-2">
          <span
            className="mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full"
            data-testid="annotation-color"
            style={{ backgroundColor: record.color || ANNOTATION_DEFAULT_COLOR }}
          />
          <div>
            <div className="font-semibold">
              {record.title}
              {record.source === 'broadcast' ? (
                <Tag className="ml-2" color="purple">{t`Broadcast`}</Tag>
              ) : null}
            </div>
            {record.description ? (
              <div className="text-sm text-gray-500">{record.description}</div>
            ) : null}
          </div>
        </div>
      )
    },
    {
      title: '',
      key: 'actions',
      width: 100,
      render: (_, record) => (
        <div className="flex gap-1">
          <Popconfirm
            title={t`Delete annotation?`}
            description={deletePrompt(record)}
            onConfirm={() => onDelete(record)}
            okText={t`Delete`}
            okButtonProps={{ danger: true }}
          >
            <Button type="text" size="small" aria-label={t`Delete`} icon={<DeleteOutlined />} />
          </Popconfirm>
          <Button
            type="text"
            size="small"
            aria-label={t`Edit`}
            icon={<EditOutlined />}
            onClick={() => onEdit(record)}
          />
        </div>
      )
    }
  ]

  return (
    <>
      {/* Mobile: card list */}
      <div className="space-y-3 md:hidden" data-testid="annotations-mobile">
        {annotations.length === 0 ? (
          <div className="rounded-lg bg-white p-6">
            <Empty description={t`No annotations yet`} image={Empty.PRESENTED_IMAGE_SIMPLE} />
          </div>
        ) : (
          annotations.map((annotation) => (
            <div
              key={annotation.id}
              className="rounded-lg border border-gray-200 bg-white p-4"
            >
              <div className="flex items-start gap-3">
                <span
                  className="mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full"
                  style={{ backgroundColor: annotation.color || ANNOTATION_DEFAULT_COLOR }}
                />
                <div className="min-w-0 flex-1">
                  <div className="font-medium">
                    {annotation.title}
                    {annotation.source === 'broadcast' ? (
                      <Tag className="ml-2" color="purple">{t`Broadcast`}</Tag>
                    ) : null}
                  </div>
                  {annotation.description ? (
                    <div className="mt-1 text-sm text-gray-500">{annotation.description}</div>
                  ) : null}
                  <div className="mt-2 text-sm text-gray-400">
                    {formatMoment(annotation)}
                    <span className="mx-1">·</span>
                    {annotation.timezone}
                  </div>
                </div>
              </div>
              <div className="mt-3 flex gap-2 border-t border-gray-100 pt-3">
                <Popconfirm
                  title={t`Delete annotation?`}
                  description={deletePrompt(annotation)}
                  onConfirm={() => onDelete(annotation)}
                  okText={t`Delete`}
                  okButtonProps={{ danger: true }}
                >
                  <Button block size="small" icon={<DeleteOutlined />}>
                    {t`Delete`}
                  </Button>
                </Popconfirm>
                <Button
                  block
                  size="small"
                  icon={<EditOutlined />}
                  onClick={() => onEdit(annotation)}
                >
                  {t`Edit`}
                </Button>
              </div>
            </div>
          ))
        )}
      </div>

      {/* Desktop: table */}
      <div className="hidden rounded-lg bg-white shadow-sm md:block" data-testid="annotations-desktop">
        <Table
          dataSource={annotations}
          columns={columns}
          rowKey="id"
          loading={loading}
          pagination={false}
          showHeader={false}
          locale={{ emptyText: t`No annotations yet` }}
        />
      </div>
    </>
  )
}
