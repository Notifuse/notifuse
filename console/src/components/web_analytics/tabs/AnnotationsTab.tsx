import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, App, Button, Segmented } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import {
  ANNOTATIONS_QUERY_KEY,
  Annotation,
  AnnotationSource,
  annotationService
} from '../../../services/api/annotation'
import { useWebAnalytics } from '../useWebAnalytics'
import { AnnotationDraft, AnnotationFormModal } from '../annotations/AnnotationFormModal'
import { AnnotationTable } from '../annotations/AnnotationTable'

const ALL_SOURCES = 'all'

type SourceFilter = typeof ALL_SOURCES | AnnotationSource

export function AnnotationsTab() {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const { workspaceId, timezone } = useWebAnalytics()

  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Annotation | undefined>()
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>(ALL_SOURCES)

  const {
    data: annotations = [],
    isLoading,
    isError
  } = useQuery({
    queryKey: [ANNOTATIONS_QUERY_KEY, workspaceId],
    // The page lists the whole set; the server clamps at the same 1000.
    queryFn: () => annotationService.list({ workspace_id: workspaceId, limit: 1000 })
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: [ANNOTATIONS_QUERY_KEY, workspaceId] })

  const reportError = (error: unknown) =>
    message.error(error instanceof Error ? error.message : String(error))

  const closeModal = () => {
    setModalOpen(false)
    setEditing(undefined)
  }

  const createMutation = useMutation({
    mutationFn: (draft: AnnotationDraft) =>
      annotationService.create({ workspace_id: workspaceId, ...draft }),
    onSuccess: async () => {
      await invalidate()
      closeModal()
      message.success(t`Annotation added`)
    },
    onError: reportError
  })

  const updateMutation = useMutation({
    mutationFn: (variables: { id: string; draft: AnnotationDraft }) =>
      annotationService.update({ workspace_id: workspaceId, id: variables.id, ...variables.draft }),
    onSuccess: async () => {
      await invalidate()
      closeModal()
      message.success(t`Annotation updated`)
    },
    onError: reportError
  })

  const deleteMutation = useMutation({
    mutationFn: (annotation: Annotation) => annotationService.delete(workspaceId, annotation.id),
    onSuccess: async () => {
      await invalidate()
      message.success(t`Annotation deleted`)
    },
    onError: reportError
  })

  const visibleAnnotations = useMemo(() => {
    if (sourceFilter === ALL_SOURCES) return annotations
    return annotations.filter((annotation) => annotation.source === sourceFilter)
  }, [annotations, sourceFilter])

  const handleSubmit = (draft: AnnotationDraft) => {
    if (editing) {
      updateMutation.mutate({ id: editing.id, draft })
      return
    }
    createMutation.mutate(draft)
  }

  const openCreate = () => {
    setEditing(undefined)
    setModalOpen(true)
  }

  const openEdit = (annotation: Annotation) => {
    setEditing(annotation)
    setModalOpen(true)
  }

  return (
    <div>
      <div className="mb-6">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-medium text-gray-800">{t`Annotations`}</h2>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            {t`Add`}
          </Button>
        </div>
        <p className="mt-3 text-gray-500 md:mt-1">
          {t`Mark the dates that explain a chart — a launch, a campaign, an outage — and they appear as labelled verticals on the dashboard and goal charts.`}
        </p>
      </div>

      <div className="mb-4">
        <Segmented
          value={sourceFilter}
          onChange={(value) => setSourceFilter(value as SourceFilter)}
          options={[
            { value: ALL_SOURCES, label: t`All` },
            { value: 'manual', label: t`Manual` },
            { value: 'broadcast', label: t`Broadcast` }
          ]}
        />
      </div>

      {isError ? (
        <Alert type="error" showIcon title={t`Could not load the annotations`} className="mb-4" />
      ) : null}

      <AnnotationTable
        annotations={visibleAnnotations}
        loading={isLoading}
        onEdit={openEdit}
        onDelete={(annotation) => deleteMutation.mutate(annotation)}
      />

      <AnnotationFormModal
        open={modalOpen}
        annotation={editing}
        defaultTimezone={timezone}
        saving={createMutation.isPending || updateMutation.isPending}
        onClose={closeModal}
        onSubmit={handleSubmit}
      />
    </div>
  )
}
