import { useLingui } from '@lingui/react/macro'
import { Tag } from 'antd'
import dayjs from '../../lib/dayjs'
import type { BlogPost } from '../../services/api/blog'

interface PostStatusTagProps {
  post: BlogPost
}

export function PostStatusTag({ post }: PostStatusTagProps) {
  const { t } = useLingui()
  if (post.published_at) {
    const publishDate = dayjs(post.published_at)
    const now = dayjs()
    const isFuture = publishDate.isAfter(now)

    if (isFuture) {
      return (
        <Tag color="orange" variant="filled">
          {t`Scheduled`}
        </Tag>
      )
    }

    return (
      <Tag color="green" variant="filled">
        {t`Published`}
      </Tag>
    )
  }
  return (
    <Tag color="blue" variant="filled">
      {t`Draft`}
    </Tag>
  )
}
