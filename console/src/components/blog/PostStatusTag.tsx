import { Tag } from 'antd'
import type { BlogPost } from '../../services/api/blog'

interface PostStatusTagProps {
  post: BlogPost
}

export function PostStatusTag({ post }: PostStatusTagProps) {
  if (post.published_at) {
    return (
      <Tag color="green" bordered={false}>
        Published
      </Tag>
    )
  }
  return (
    <Tag color="blue" bordered={false}>
      Draft
    </Tag>
  )
}
