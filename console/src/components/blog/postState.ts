import type { BlogPost } from '../../services/api/blog'

/** Where a post stands: a scheduled post has a publication date still to come. */
export type PostState = 'draft' | 'scheduled' | 'published'

export function postState(post: BlogPost, now: Date = new Date()): PostState {
  if (!post.published_at) return 'draft'
  return new Date(post.published_at).getTime() > now.getTime() ? 'scheduled' : 'published'
}

/** The path the public blog serves a post at, and web analytics records. */
export function postPath(categorySlug: string, postSlug: string): string {
  return `/${categorySlug}/${postSlug}`
}
