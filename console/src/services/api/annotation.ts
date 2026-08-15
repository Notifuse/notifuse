import { api } from './client'

// Types mirror internal/domain/annotation.go.

export type AnnotationSource = 'manual' | 'broadcast'

export interface Annotation {
  id: string
  /** RFC3339 instant. The moment the annotation marks, independent of `timezone`. */
  annotated_at: string
  /**
   * Display intent only: it is what lets "9am in Tokyo" render back as 9am
   * rather than as the reader's local equivalent. Never use it to re-parse
   * `annotated_at`, which already fixes the instant.
   */
  timezone: string
  title: string
  description?: string
  color: string
  source: AnnotationSource
  /** Set on automatic rows only — the id of the entity that caused them. */
  source_id?: string
  created_at: string
  updated_at: string
}

export interface ListAnnotationsParams {
  workspace_id: string
  /** RFC3339. */
  start?: string
  /** RFC3339. */
  end?: string
  sources?: AnnotationSource[]
  limit?: number
}

export interface CreateAnnotationRequest {
  workspace_id: string
  annotated_at: string
  timezone?: string
  title: string
  description?: string
  color?: string
}

export interface UpdateAnnotationRequest {
  workspace_id: string
  id: string
  annotated_at: string
  timezone?: string
  title: string
  description?: string
  color?: string
}

interface ListAnnotationsResponse {
  annotations: Annotation[]
}

interface AnnotationResponse {
  annotation: Annotation
}

/**
 * Query key prefix. Callers append the workspace id, and the range for the
 * chart queries, so invalidating `[ANNOTATIONS_QUERY_KEY, workspaceId]`
 * prefix-matches every open range.
 */
export const ANNOTATIONS_QUERY_KEY = 'annotations'

/**
 * Mirrors `domain.AnnotationMaxListLimit`. Asking for it is what keeps a caller
 * off the endpoint's default of 100, which is applied after ordering by
 * `annotated_at` descending and so silently drops the oldest rows.
 */
export const ANNOTATION_LIST_MAX = 1000

export const annotationService = {
  async list(params: ListAnnotationsParams): Promise<Annotation[]> {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)

    if (params.start) searchParams.append('start', params.start)
    if (params.end) searchParams.append('end', params.end)
    // The endpoint splits this on commas; repeated params are not read.
    if (params.sources && params.sources.length > 0) {
      searchParams.append('sources', params.sources.join(','))
    }
    if (params.limit) searchParams.append('limit', params.limit.toString())

    const response = await api.get<ListAnnotationsResponse>(
      `/api/annotations.list?${searchParams.toString()}`
    )
    return response.annotations
  },

  async get(workspaceId: string, id: string): Promise<Annotation> {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', workspaceId)
    searchParams.append('id', id)

    const response = await api.get<AnnotationResponse>(
      `/api/annotations.get?${searchParams.toString()}`
    )
    return response.annotation
  },

  async create(params: CreateAnnotationRequest): Promise<Annotation> {
    const response = await api.post<AnnotationResponse>('/api/annotations.create', params)
    return response.annotation
  },

  async update(params: UpdateAnnotationRequest): Promise<Annotation> {
    const response = await api.post<AnnotationResponse>('/api/annotations.update', params)
    return response.annotation
  },

  async delete(workspaceId: string, id: string): Promise<void> {
    await api.post('/api/annotations.delete', { workspace_id: workspaceId, id })
  }
}
