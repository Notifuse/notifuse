import { api } from './client'

// SEO Settings types
export interface SEOSettings {
  meta_title?: string
  meta_description?: string
  og_title?: string
  og_description?: string
  og_image?: string
  canonical_url?: string
  keywords?: string[]
  meta_robots?: string
}

// Blog Author types
export interface BlogAuthor {
  name: string
  avatar_url?: string
}

// Blog Category types
export interface BlogCategorySettings {
  name: string
  description?: string
  seo?: SEOSettings
}

export interface BlogCategory {
  id: string
  slug: string
  settings: BlogCategorySettings
  created_at: string
  updated_at: string
  deleted_at?: string
}

export interface CreateBlogCategoryRequest {
  name: string
  slug: string
  description?: string
  seo?: SEOSettings
}

export interface UpdateBlogCategoryRequest {
  id: string
  name: string
  slug: string
  description?: string
  seo?: SEOSettings
}

export interface DeleteBlogCategoryRequest {
  id: string
}

export interface GetBlogCategoryRequest {
  id?: string
  slug?: string
}

export interface BlogCategoryListResponse {
  categories: BlogCategory[]
  total_count: number
}

// Blog Post types
export interface BlogPostTemplateReference {
  template_id: string
  template_version: number
}

export interface BlogPostSettings {
  title: string
  template: BlogPostTemplateReference
  excerpt?: string
  featured_image_url?: string
  authors: BlogAuthor[]
  reading_time_minutes: number
  seo?: SEOSettings
}

export interface BlogPost {
  id: string
  category_id?: string | null
  slug: string
  settings: BlogPostSettings
  published_at?: string | null
  created_at: string
  updated_at: string
  deleted_at?: string
}

export interface CreateBlogPostRequest {
  category_id?: string | null
  slug: string
  title: string
  template_id: string
  template_version: number
  excerpt?: string
  featured_image_url?: string
  authors: BlogAuthor[]
  reading_time_minutes: number
  seo?: SEOSettings
}

export interface UpdateBlogPostRequest {
  id: string
  category_id?: string | null
  slug: string
  title: string
  template_id: string
  template_version: number
  excerpt?: string
  featured_image_url?: string
  authors: BlogAuthor[]
  reading_time_minutes: number
  seo?: SEOSettings
}

export interface DeleteBlogPostRequest {
  id: string
}

export interface PublishBlogPostRequest {
  id: string
  published_at?: string
  timezone?: string
}

export interface UnpublishBlogPostRequest {
  id: string
}

export interface GetBlogPostRequest {
  id?: string
  slug?: string
  category_slug?: string
}

export type BlogPostStatus = 'all' | 'draft' | 'published'

export interface ListBlogPostsRequest {
  category_id?: string
  status?: BlogPostStatus
  limit?: number
  offset?: number
}

export interface BlogPostListResponse {
  posts: BlogPost[]
  total_count: number
}

// Response wrappers
export interface GetBlogCategoryResponse {
  category: BlogCategory
}

export interface CreateBlogCategoryResponse {
  category: BlogCategory
}

export interface UpdateBlogCategoryResponse {
  category: BlogCategory
}

export interface DeleteBlogCategoryResponse {
  success: boolean
}

export interface GetBlogPostResponse {
  post: BlogPost
}

export interface CreateBlogPostResponse {
  post: BlogPost
}

export interface UpdateBlogPostResponse {
  post: BlogPost
}

export interface DeleteBlogPostResponse {
  success: boolean
}

export interface PublishBlogPostResponse {
  success: boolean
}

export interface UnpublishBlogPostResponse {
  success: boolean
}

// Category API
export interface BlogCategoriesApi {
  list: (workspace_id: string) => Promise<BlogCategoryListResponse>
  get: (workspace_id: string, params: GetBlogCategoryRequest) => Promise<GetBlogCategoryResponse>
  create: (
    workspace_id: string,
    params: CreateBlogCategoryRequest
  ) => Promise<CreateBlogCategoryResponse>
  update: (
    workspace_id: string,
    params: UpdateBlogCategoryRequest
  ) => Promise<UpdateBlogCategoryResponse>
  delete: (
    workspace_id: string,
    params: DeleteBlogCategoryRequest
  ) => Promise<DeleteBlogCategoryResponse>
}

export const blogCategoriesApi: BlogCategoriesApi = {
  list: async (workspace_id: string): Promise<BlogCategoryListResponse> => {
    const url = `/api/blogCategories.list?workspace_id=${workspace_id}`
    return await api.get<BlogCategoryListResponse>(url)
  },

  get: async (
    workspace_id: string,
    params: GetBlogCategoryRequest
  ): Promise<GetBlogCategoryResponse> => {
    let url = `/api/blogCategories.get?workspace_id=${workspace_id}`
    if (params.id) {
      url += `&id=${params.id}`
    }
    if (params.slug) {
      url += `&slug=${params.slug}`
    }
    return await api.get<GetBlogCategoryResponse>(url)
  },

  create: async (
    workspace_id: string,
    params: CreateBlogCategoryRequest
  ): Promise<CreateBlogCategoryResponse> => {
    const url = `/api/blogCategories.create?workspace_id=${workspace_id}`
    return await api.post<CreateBlogCategoryResponse>(url, params)
  },

  update: async (
    workspace_id: string,
    params: UpdateBlogCategoryRequest
  ): Promise<UpdateBlogCategoryResponse> => {
    const url = `/api/blogCategories.update?workspace_id=${workspace_id}`
    return await api.post<UpdateBlogCategoryResponse>(url, params)
  },

  delete: async (
    workspace_id: string,
    params: DeleteBlogCategoryRequest
  ): Promise<DeleteBlogCategoryResponse> => {
    const url = `/api/blogCategories.delete?workspace_id=${workspace_id}`
    return await api.post<DeleteBlogCategoryResponse>(url, params)
  }
}

// Posts API
export interface BlogPostsApi {
  list: (workspace_id: string, params: ListBlogPostsRequest) => Promise<BlogPostListResponse>
  get: (workspace_id: string, params: GetBlogPostRequest) => Promise<GetBlogPostResponse>
  create: (workspace_id: string, params: CreateBlogPostRequest) => Promise<CreateBlogPostResponse>
  update: (workspace_id: string, params: UpdateBlogPostRequest) => Promise<UpdateBlogPostResponse>
  delete: (workspace_id: string, params: DeleteBlogPostRequest) => Promise<DeleteBlogPostResponse>
  publish: (
    workspace_id: string,
    params: PublishBlogPostRequest
  ) => Promise<PublishBlogPostResponse>
  unpublish: (
    workspace_id: string,
    params: UnpublishBlogPostRequest
  ) => Promise<UnpublishBlogPostResponse>
}

export const blogPostsApi: BlogPostsApi = {
  list: async (
    workspace_id: string,
    params: ListBlogPostsRequest
  ): Promise<BlogPostListResponse> => {
    let url = `/api/blogPosts.list?workspace_id=${workspace_id}`
    if (params.category_id) {
      url += `&category_id=${params.category_id}`
    }
    if (params.status) {
      url += `&status=${params.status}`
    }
    if (params.limit) {
      url += `&limit=${params.limit}`
    }
    if (params.offset) {
      url += `&offset=${params.offset}`
    }
    return await api.get<BlogPostListResponse>(url)
  },

  get: async (workspace_id: string, params: GetBlogPostRequest): Promise<GetBlogPostResponse> => {
    let url = `/api/blogPosts.get?workspace_id=${workspace_id}`
    if (params.id) {
      url += `&id=${params.id}`
    }
    if (params.slug) {
      url += `&slug=${params.slug}`
    }
    if (params.category_slug) {
      url += `&category_slug=${params.category_slug}`
    }
    return await api.get<GetBlogPostResponse>(url)
  },

  create: async (
    workspace_id: string,
    params: CreateBlogPostRequest
  ): Promise<CreateBlogPostResponse> => {
    const url = `/api/blogPosts.create?workspace_id=${workspace_id}`
    return await api.post<CreateBlogPostResponse>(url, params)
  },

  update: async (
    workspace_id: string,
    params: UpdateBlogPostRequest
  ): Promise<UpdateBlogPostResponse> => {
    const url = `/api/blogPosts.update?workspace_id=${workspace_id}`
    return await api.post<UpdateBlogPostResponse>(url, params)
  },

  delete: async (
    workspace_id: string,
    params: DeleteBlogPostRequest
  ): Promise<DeleteBlogPostResponse> => {
    const url = `/api/blogPosts.delete?workspace_id=${workspace_id}`
    return await api.post<DeleteBlogPostResponse>(url, params)
  },

  publish: async (
    workspace_id: string,
    params: PublishBlogPostRequest
  ): Promise<PublishBlogPostResponse> => {
    const url = `/api/blogPosts.publish?workspace_id=${workspace_id}`
    return await api.post<PublishBlogPostResponse>(url, params)
  },

  unpublish: async (
    workspace_id: string,
    params: UnpublishBlogPostRequest
  ): Promise<UnpublishBlogPostResponse> => {
    const url = `/api/blogPosts.unpublish?workspace_id=${workspace_id}`
    return await api.post<UnpublishBlogPostResponse>(url, params)
  }
}

// Blog Theme types
export interface BlogThemeFiles {
  'home.liquid': string
  'category.liquid': string
  'post.liquid': string
  'header.liquid': string
  'footer.liquid': string
  'shared.liquid': string
  'styles.css': string
  'scripts.js': string
}

export interface BlogTheme {
  version: number
  published_at?: string | null
  files: BlogThemeFiles
  notes?: string
  created_at: string
  updated_at: string
}

export interface CreateBlogThemeRequest {
  files: BlogThemeFiles
  notes?: string
}

export interface UpdateBlogThemeRequest {
  version: number
  files: BlogThemeFiles
  notes?: string
}

export interface PublishBlogThemeRequest {
  version: number
}

export interface GetBlogThemeRequest {
  version: number
}

export interface ListBlogThemesRequest {
  limit?: number
  offset?: number
}

export interface BlogThemeListResponse {
  themes: BlogTheme[]
  total_count: number
}

export interface GetBlogThemeResponse {
  theme: BlogTheme
}

export interface CreateBlogThemeResponse {
  theme: BlogTheme
}

export interface UpdateBlogThemeResponse {
  theme: BlogTheme
}

export interface PublishBlogThemeResponse {
  success: boolean
  message: string
}

export interface GetPublishedBlogThemeResponse {
  theme: BlogTheme
}

// Theme API
export interface BlogThemesApi {
  list: (workspace_id: string, params: ListBlogThemesRequest) => Promise<BlogThemeListResponse>
  get: (workspace_id: string, version: number) => Promise<GetBlogThemeResponse>
  getPublished: (workspace_id: string) => Promise<GetPublishedBlogThemeResponse>
  create: (workspace_id: string, params: CreateBlogThemeRequest) => Promise<CreateBlogThemeResponse>
  update: (workspace_id: string, params: UpdateBlogThemeRequest) => Promise<UpdateBlogThemeResponse>
  publish: (
    workspace_id: string,
    params: PublishBlogThemeRequest
  ) => Promise<PublishBlogThemeResponse>
}

export const blogThemesApi: BlogThemesApi = {
  list: async (
    workspace_id: string,
    params: ListBlogThemesRequest
  ): Promise<BlogThemeListResponse> => {
    let url = `/api/blogThemes.list?workspace_id=${workspace_id}`
    if (params.limit) {
      url += `&limit=${params.limit}`
    }
    if (params.offset) {
      url += `&offset=${params.offset}`
    }
    return await api.get<BlogThemeListResponse>(url)
  },

  get: async (workspace_id: string, version: number): Promise<GetBlogThemeResponse> => {
    const url = `/api/blogThemes.get?workspace_id=${workspace_id}&version=${version}`
    return await api.get<GetBlogThemeResponse>(url)
  },

  getPublished: async (workspace_id: string): Promise<GetPublishedBlogThemeResponse> => {
    const url = `/api/blogThemes.getPublished?workspace_id=${workspace_id}`
    return await api.get<GetPublishedBlogThemeResponse>(url)
  },

  create: async (
    workspace_id: string,
    params: CreateBlogThemeRequest
  ): Promise<CreateBlogThemeResponse> => {
    const url = `/api/blogThemes.create?workspace_id=${workspace_id}`
    return await api.post<CreateBlogThemeResponse>(url, params)
  },

  update: async (
    workspace_id: string,
    params: UpdateBlogThemeRequest
  ): Promise<UpdateBlogThemeResponse> => {
    const url = `/api/blogThemes.update?workspace_id=${workspace_id}`
    return await api.post<UpdateBlogThemeResponse>(url, params)
  },

  publish: async (
    workspace_id: string,
    params: PublishBlogThemeRequest
  ): Promise<PublishBlogThemeResponse> => {
    const url = `/api/blogThemes.publish?workspace_id=${workspace_id}`
    return await api.post<PublishBlogThemeResponse>(url, params)
  }
}

// Utility function to normalize slug (matches backend logic)
export function normalizeSlug(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[\s_]+/g, '-') // Replace spaces and underscores with hyphens
    .replace(/[^a-z0-9-]/g, '') // Remove any characters that aren't lowercase letters, numbers, or hyphens
    .replace(/-+/g, '-') // Replace multiple hyphens with single hyphen
    .replace(/^-|-$/g, '') // Remove leading/trailing hyphens
}
