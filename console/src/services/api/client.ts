import config from '../../config'

class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public data?: any
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const errorData = await response.json().catch(() => null)
    throw new ApiError(errorData?.error || 'An error occurred', response.status, errorData)
  }
  return response.json()
}

async function request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const authToken = localStorage.getItem('auth_token')
  const headers = {
    'Content-Type': 'application/json',
    ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
    ...options.headers
  }

  const response = await fetch(`${config.API_ENDPOINT}${endpoint}`, {
    ...options,
    headers
  })

  return handleResponse<T>(response)
}

export const api = {
  get: <T>(endpoint: string) => request<T>(endpoint),
  post: <T>(endpoint: string, data: any) =>
    request<T>(endpoint, {
      method: 'POST',
      body: JSON.stringify(data)
    }),
  put: <T>(endpoint: string, data: any) =>
    request<T>(endpoint, {
      method: 'PUT',
      body: JSON.stringify(data)
    }),
  delete: <T>(endpoint: string) =>
    request<T>(endpoint, {
      method: 'DELETE'
    })
}
