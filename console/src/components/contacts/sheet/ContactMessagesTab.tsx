import type { Workspace } from '../../../services/api/types'
import { MessageHistoryTable } from '../../messages/MessageHistoryTable'
import { BlockError } from '../BlockError'
import { useContactMessages } from './useContactSheetQueries'

/** Every message sent to the contact, page by page. */
export function ContactMessagesTab({ workspace, email }: { workspace: Workspace; email: string }) {
  const messages = useContactMessages(workspace.id, email, true)
  if (messages.isError && !messages.data) return <BlockError onRetry={() => messages.refetch()} />
  const pages = messages.data?.pages ?? []
  return (
    <MessageHistoryTable
      messages={pages.flatMap((page) => page.messages ?? [])}
      loading={messages.isLoading}
      isLoadingMore={messages.isFetchingNextPage}
      nextCursor={pages[pages.length - 1]?.next_cursor}
      onLoadMore={() => messages.fetchNextPage()}
      workspace={workspace}
      show_email={false}
      size="small"
    />
  )
}
