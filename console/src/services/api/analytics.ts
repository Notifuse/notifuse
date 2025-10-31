import { api } from './client'

// Analytics types
export interface AnalyticsQuery {
  schema: string // Predefined schema name (required)
  measures: string[] // Aggregation fields (count, sum, avg, etc.)
  dimensions: string[] // Grouping fields
  timezone?: string // Timezone for date/time operations (e.g., "America/New_York", "UTC")
  timeDimensions?: {
    dimension: string // Date/timestamp field
    granularity: 'hour' | 'day' | 'week' | 'month' | 'year'
    dateRange?: [string, string] | null // ISO date strings - array of exactly 2 strings or null
  }[]
  filters?: {
    member: string // Field name
    operator:
      | 'equals'
      | 'notEquals'
      | 'contains'
      | 'notContains'
      | 'startsWith'
      | 'notStartsWith'
      | 'endsWith'
      | 'notEndsWith'
      | 'gt'
      | 'gte'
      | 'lt'
      | 'lte'
      | 'in'
      | 'notIn'
      | 'set'
      | 'notSet'
      | 'inDateRange'
      | 'notInDateRange'
      | 'beforeDate'
      | 'afterDate'
    values: string[]
  }[]
  limit?: number // Result limit (default: 1000)
  offset?: number // Pagination offset
  order?: {
    [key: string]: 'asc' | 'desc' // Sorting
  }
}

// Request payload structure matching backend expectations
export interface AnalyticsQueryRequest {
  workspace_id: string
  query: AnalyticsQuery
}

export interface AnalyticsResponse {
  data: Array<Record<string, any>>
  meta: {
    total: number
    executionTime?: number
    query: string // Generated SQL for debugging
    params: any[] // Database parameters for debugging
  }
}

export interface AnalyticsError {
  error: string
  message?: string
}

// Cache item for storing analytics responses
interface CacheItem {
  response: AnalyticsResponse
  timestamp: number
  ttl: number
}

// Queue item for the analytics service
interface QueueItem {
  query: AnalyticsQueryRequest
  resolve: (response: AnalyticsResponse) => void
  reject: (error: any) => void
}

// Analytics Service Configuration
interface AnalyticsServiceConfig {
  maxConcurrency?: number
  cacheTTL?: number // Cache time-to-live in milliseconds (default: 30000ms = 30 seconds)
}

// Singleton Analytics Service Class
class AnalyticsService {
  private static instance: AnalyticsService
  private queue: QueueItem[] = []
  private activeRequests: number = 0
  private maxConcurrency: number = 1
  private cache: Map<string, CacheItem> = new Map()
  private cacheTTL: number = 30000 // 30 seconds default

  private constructor(config: AnalyticsServiceConfig = {}) {
    this.maxConcurrency = config.maxConcurrency ?? 1
    this.cacheTTL = config.cacheTTL ?? 30000
  }

  public static getInstance(config?: AnalyticsServiceConfig): AnalyticsService {
    if (!AnalyticsService.instance) {
      AnalyticsService.instance = new AnalyticsService(config)
    }
    return AnalyticsService.instance
  }

  public configure(config: AnalyticsServiceConfig): void {
    this.maxConcurrency = config.maxConcurrency ?? this.maxConcurrency
    this.cacheTTL = config.cacheTTL ?? this.cacheTTL
  }

  private generateCacheKey(queryRequest: AnalyticsQueryRequest): string {
    return JSON.stringify(queryRequest)
  }

  private isCacheValid(cacheItem: CacheItem): boolean {
    const now = Date.now()
    return now - cacheItem.timestamp < cacheItem.ttl
  }

  private getCachedResponse(cacheKey: string): AnalyticsResponse | null {
    const cacheItem = this.cache.get(cacheKey)
    if (!cacheItem) {
      return null
    }

    if (this.isCacheValid(cacheItem)) {
      return cacheItem.response
    }

    // Remove expired cache item
    this.cache.delete(cacheKey)
    return null
  }

  private setCachedResponse(cacheKey: string, response: AnalyticsResponse): void {
    const cacheItem: CacheItem = {
      response,
      timestamp: Date.now(),
      ttl: this.cacheTTL
    }
    this.cache.set(cacheKey, cacheItem)
  }

  public async query(query: AnalyticsQuery, workspaceId: string): Promise<AnalyticsResponse> {
    const queryRequest: AnalyticsQueryRequest = { workspace_id: workspaceId, query }
    const cacheKey = this.generateCacheKey(queryRequest)

    // Check cache first
    const cachedResponse = this.getCachedResponse(cacheKey)
    if (cachedResponse) {
      return Promise.resolve(cachedResponse)
    }

    return new Promise<AnalyticsResponse>((resolve, reject) => {
      // Store both query and workspaceId for the queue item
      this.queue.push({
        query: queryRequest,
        resolve: (response: AnalyticsResponse) => {
          // Cache the response before resolving
          this.setCachedResponse(cacheKey, response)
          resolve(response)
        },
        reject
      })
      this.processQueue()
    })
  }

  private async processQueue(): Promise<void> {
    if (this.activeRequests >= this.maxConcurrency || this.queue.length === 0) {
      return
    }

    const item = this.queue.shift()
    if (!item) return

    this.activeRequests++

    try {
      const response = await this.executeQuery(item.query)
      item.resolve(response)
    } catch (error) {
      item.reject(error)
    } finally {
      this.activeRequests--
      // Process next item in queue
      if (this.queue.length > 0) {
        setTimeout(() => this.processQueue(), 0)
      }
    }
  }

  private async executeQuery(queryRequest: AnalyticsQueryRequest): Promise<AnalyticsResponse> {
    return api.post<AnalyticsResponse>('/api/analytics.query', queryRequest)
  }

  // Clear expired cache entries
  private cleanupExpiredCache(): void {
    const now = Date.now()
    for (const [key, cacheItem] of this.cache.entries()) {
      if (now - cacheItem.timestamp >= cacheItem.ttl) {
        this.cache.delete(key)
      }
    }
  }

  // Clear all cache entries
  public clearCache(): void {
    this.cache.clear()
  }

  // Get current queue status for debugging
  public getQueueStatus(): { queueLength: number; activeRequests: number; maxConcurrency: number } {
    return {
      queueLength: this.queue.length,
      activeRequests: this.activeRequests,
      maxConcurrency: this.maxConcurrency
    }
  }

  // Get current cache status for debugging
  public getCacheStatus(): { cacheSize: number; cacheTTL: number } {
    // Clean up expired entries before reporting
    this.cleanupExpiredCache()
    return {
      cacheSize: this.cache.size,
      cacheTTL: this.cacheTTL
    }
  }
}

// Export singleton instance
export const analyticsService = AnalyticsService.getInstance()

// Export the class for configuration
export { AnalyticsService }
