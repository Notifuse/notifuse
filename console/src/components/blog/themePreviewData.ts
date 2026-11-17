import type { BlogCategory } from '../../services/api/blog'
import type { Workspace } from '../../services/api/types'
import type { Category, MockBlogData, PostListItem } from '../../utils/mockBlogData'
import { blogBaseUrl } from './category/categorySeo'

/**
 * The sample blog the theme preview renders, put in the workspace's own frame: its name, address
 * and blog settings, and its categories in place of the sample ones. The posts stay samples (a
 * theme needs a full set of blocks to be judged), each moved into one of the real categories.
 */
export function withWorkspaceData(
  data: MockBlogData,
  workspace: Workspace | null | undefined,
  categories: BlogCategory[] | undefined,
  apiEndpoint: string | undefined
): MockBlogData {
  if (!workspace) return data

  const blogSettings = workspace.settings?.blog_settings
  const baseUrl = blogBaseUrl(workspace.settings?.custom_endpoint_url, apiEndpoint)
  const next: MockBlogData = {
    ...data,
    workspace: {
      ...data.workspace,
      id: workspace.id,
      name: workspace.name,
      blog_title: blogSettings?.title || workspace.name,
      ...(blogSettings?.logo_url ? { logo_url: blogSettings.logo_url } : {}),
      ...(blogSettings?.icon_url ? { icon_url: blogSettings.icon_url } : {})
    },
    blog: data.blog ? { ...data.blog, title: blogSettings?.title || workspace.name } : undefined,
    // The sample's titles and address name a blog that is not this one: the blog's own SEO first,
    // its title and address otherwise.
    seo: {
      ...data.seo,
      meta_title: blogSettings?.title || workspace.name,
      og_title: blogSettings?.title || workspace.name,
      canonical_url: baseUrl,
      keywords: [],
      ...blogSettings?.seo
    },
    base_url: baseUrl
  }

  const real: Category[] = (categories ?? [])
    .filter((category) => !category.deleted_at)
    .map((category) => ({
      id: category.id,
      slug: category.slug,
      name: category.settings.name,
      description: category.settings.description ?? '',
      seo: category.settings.seo
    }))

  if (real.length > 0) {
    // Each sample category stands for a real one, in order, cycling when the blog has fewer.
    const stand = new Map(data.categories.map((sample, index) => [sample.id, real[index % real.length]]))
    const moved = (categoryId: string) => stand.get(categoryId) ?? real[0]
    const movePost = <T extends PostListItem>(post: T): T => {
      const category = moved(post.category_id)
      return { ...post, category_id: category.id, category_slug: category.slug }
    }

    next.categories = real
    next.posts = data.posts.map(movePost)
    if (data.previous_post) next.previous_post = movePost(data.previous_post)
    if (data.next_post) next.next_post = movePost(data.next_post)
    if (data.category) {
      const category = moved(data.category.id)
      next.category = category
      if (!data.post) {
        // A category page lists the posts of its category.
        next.posts = next.posts.filter((post) => post.category_id === category.id)
        next.page_description = category.description
      }
    }
    if (data.post) next.post = movePost(data.post)
    if (next.pagination) {
      next.pagination = {
        ...next.pagination,
        total_count: next.posts.length,
        total_pages: Math.max(1, Math.ceil(next.posts.length / next.pagination.per_page))
      }
    }
  }

  // The page's address and title, rebuilt from the real ones.
  const blogTitle = next.workspace.blog_title || next.workspace.name
  if (next.post) {
    next.current_url = `${baseUrl}/${next.post.category_slug}/${next.post.slug}`
    next.page_title = `${next.post.title} - ${blogTitle}`
  } else if (next.category) {
    next.current_url = `${baseUrl}/${next.category.slug}`
    next.page_title = `${next.category.name} - ${blogTitle}`
  } else {
    next.current_url = baseUrl
  }
  return next
}
