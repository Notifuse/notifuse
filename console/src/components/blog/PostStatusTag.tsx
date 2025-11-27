import { Tag } from 'antd'
import dayjs from '../../lib/dayjs'
import type { BlogPost } from '../../services/api/blog'

interface PostStatusTagProps {
  post: BlogPost
}

export function PostStatusTag({ post }: PostStatusTagProps) {
  if (post.published_at) {
    const publishDate = dayjs(post.published_at)
    const now = dayjs()
    const isFuture = publishDate.isAfter(now)

    if (isFuture) {
      return (
        <Tag color="orange" bordered={false}>
          Scheduled
        </Tag>
      )
    }

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
