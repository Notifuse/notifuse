import { api } from './client'

// Task status types
export type TaskStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'paused'

// State interfaces
export interface SendBroadcastState {
  broadcast_id: string
  total_recipients: number
  sent_count: number
  failed_count: number
  channel_type: string
  recipient_offset: number
}

export interface BuildSegmentState {
  segment_id: string
  version: number
  total_contacts: number
  processed_count: number
  matched_count: number
  contact_offset: number
  batch_size: number
  started_at: string
}

export interface TaskState {
  progress?: number
  message?: string
  send_broadcast?: SendBroadcastState
  build_segment?: BuildSegmentState
}

// Task interfaces
export interface Task {
  id: string
  workspace_id: string
  type: string
  status: TaskStatus
  progress: number
  state?: TaskState
  error_message?: string
  created_at: string
  updated_at: string
  last_run_at?: string
  completed_at?: string
  next_run_after?: string
  timeout_after?: string
  max_runtime: number
  max_retries: number
  retry_count: number
  retry_interval: number
  broadcast_id?: string
}

// API request interfaces
export interface CreateTaskRequest {
  workspace_id: string
  type: string
  state?: TaskState
  max_runtime?: number
  max_retries?: number
  retry_interval?: number
  next_run_after?: string
}

export interface GetTaskRequest {
  workspace_id: string
  id: string
}

export interface DeleteTaskRequest {
  workspace_id: string
  id: string
}

export interface ExecuteTaskRequest {
  workspace_id: string
  id: string
}

export interface ListTasksRequest {
  workspace_id: string
  status?: TaskStatus | TaskStatus[]
  type?: string | string[]
  created_after?: string
  created_before?: string
  limit?: number
  offset?: number
}

// API response interfaces
export interface GetTaskResponse {
  task: Task
}

export interface CreateTaskResponse {
  task: Task
}

export interface ListTasksResponse {
  tasks: Task[]
  total_count: number
  limit: number
  offset: number
  has_more: boolean
}

export interface DeleteTaskResponse {
  success: boolean
}

export interface ExecuteTaskResponse {
  success: boolean
  message: string
}

// Task API client
export const taskApi = {
  // Create a new task
  create: async (params: CreateTaskRequest): Promise<CreateTaskResponse> => {
    return api.post<CreateTaskResponse>('/api/tasks.create', params)
  },

  // Get task by ID
  get: async (params: GetTaskRequest): Promise<GetTaskResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('id', params.id)

    return api.get<GetTaskResponse>(`/api/tasks.get?${searchParams.toString()}`)
  },

  // List tasks with filtering
  list: async (params: ListTasksRequest): Promise<ListTasksResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)

    if (params.status) {
      const statusList = Array.isArray(params.status) ? params.status : [params.status]
      searchParams.append('status', statusList.join(','))
    }

    if (params.type) {
      const typeList = Array.isArray(params.type) ? params.type : [params.type]
      searchParams.append('type', typeList.join(','))
    }

    if (params.created_after) searchParams.append('created_after', params.created_after)
    if (params.created_before) searchParams.append('created_before', params.created_before)
    if (params.limit) searchParams.append('limit', params.limit.toString())
    if (params.offset) searchParams.append('offset', params.offset.toString())

    return api.get<ListTasksResponse>(`/api/tasks.list?${searchParams.toString()}`)
  },

  // Delete a task
  delete: async (params: DeleteTaskRequest): Promise<DeleteTaskResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('id', params.id)

    return api.post<DeleteTaskResponse>(`/api/tasks.delete?${searchParams.toString()}`, {})
  },

  // Execute a task
  execute: async (params: ExecuteTaskRequest): Promise<ExecuteTaskResponse> => {
    return api.post<ExecuteTaskResponse>('/api/tasks.execute', params)
  },

  // Utility method to find a task by broadcast ID (uses list with filter)
  findByBroadcastId: async (workspace_id: string, broadcast_id: string): Promise<Task | null> => {
    // We'll use the list endpoint and implement client-side filtering
    // since there's no specific getByBroadcastId endpoint
    try {
      const response = await taskApi.list({
        workspace_id,
        // Use a large limit to ensure we find the task
        limit: 100
      })

      // Find the task with the matching broadcast_id
      const task = response.tasks?.find((task) => task.broadcast_id === broadcast_id)
      return task || null
    } catch (error) {
      console.error('Error finding task by broadcast ID:', error)
      return null
    }
  },

  // Utility method to find a task by segment ID (uses list with filter)
  findBySegmentId: async (workspace_id: string, segment_id: string): Promise<Task | null> => {
    try {
      const response = await taskApi.list({
        workspace_id,
        type: 'build_segment',
        status: ['pending', 'running'],
        limit: 100
      })

      // Find the task with the matching segment_id in its build_segment state
      const task = response.tasks?.find(
        (task) => task.state?.build_segment?.segment_id === segment_id
      )
      return task || null
    } catch (error) {
      console.error('Error finding task by segment ID:', error)
      return null
    }
  }
}
