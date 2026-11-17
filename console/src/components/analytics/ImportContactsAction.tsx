import type { ReactNode } from 'react'
import { Button } from 'antd'
import { Upload } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { listsApi } from '../../services/api/list'
import { useContactsCsvUpload } from '../contacts/ContactsCsvUploadProvider'

interface ImportContactsActionProps {
  workspaceId: string
  /** Offers the workspace's lists in the import, as the Contacts page does; without it, none. */
  canReadLists: boolean
  children: ReactNode
  type?: 'primary' | 'default'
  size?: 'small' | 'middle'
  /** Runs before the drawer opens (the checklist moves to the Contacts page first). */
  beforeOpen?: () => void
}

/**
 * Opens the CSV import with the workspace's lists, as the Contacts page opens it. Its own component,
 * so the lists are asked for only where the button shows.
 */
export function ImportContactsAction({
  workspaceId,
  canReadLists,
  children,
  type = 'default',
  size = 'small',
  beforeOpen
}: ImportContactsActionProps) {
  const { openDrawer } = useContactsCsvUpload()
  // The key and answer of useListsQuery (contacts/useContactsLookups), which the Contacts page
  // reads: one cache entry. Not imported, for that module's imports (segments, the API client).
  const lists = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: canReadLists
  })
  return (
    <Button
      type={type}
      size={size}
      icon={<Upload size={14} aria-hidden="true" />}
      // The import drawer lists the targets once; it waits for them rather than open without.
      loading={canReadLists && lists.isLoading}
      onClick={() => {
        beforeOpen?.()
        openDrawer(workspaceId, lists.data?.lists ?? [], true)
      }}
    >
      {children}
    </Button>
  )
}
