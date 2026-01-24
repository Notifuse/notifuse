import React, { useState } from 'react'
import { Button } from 'antd'
import { UploadOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { List } from '../../services/api/types'
import { ContactsCsvUploadDrawer } from './ContactsCsvUploadDrawer'
import { useQueryClient } from '@tanstack/react-query'

// Create a context for the singleton
// eslint-disable-next-line react-refresh/only-export-components -- Context co-located with provider
export const CsvUploadContext = React.createContext<{
  openDrawer: (workspaceId: string, lists?: List[], refreshOnClose?: boolean) => void
  openDrawerWithSelectedList: (
    workspaceId: string,
    lists?: List[],
    selectedList?: string,
    refreshOnClose?: boolean
  ) => void
} | null>(null)

interface ContactsCsvUploadDrawerProviderProps {
  children: React.ReactNode
}

export const ContactsCsvUploadProvider: React.FC<ContactsCsvUploadDrawerProviderProps> = ({
  children
}) => {
  const [drawerVisible, setDrawerVisible] = useState(false)
  const [contextLists, setContextLists] = useState<List[]>([])
  const [contextWorkspaceId, setContextWorkspaceId] = useState<string>('')
  const [selectedList, setSelectedList] = useState<string | undefined>(undefined)
  const [shouldRefreshOnClose, setShouldRefreshOnClose] = useState(false)
  const queryClient = useQueryClient()

  // Handler for when import is successful
  const handleImportSuccess = () => {
    // Invalidate contacts, lists, and list-stats queries to trigger a refresh
    if (shouldRefreshOnClose && contextWorkspaceId) {
      queryClient.invalidateQueries({ queryKey: ['contacts', contextWorkspaceId] })
      queryClient.invalidateQueries({ queryKey: ['lists', contextWorkspaceId] })
      queryClient.invalidateQueries({ queryKey: ['list-stats', contextWorkspaceId] })
    }
  }

  const handleDrawerClose = () => {
    setDrawerVisible(false)
  }

  const openDrawer = (workspaceId: string, lists: List[] = [], refreshOnClose = true) => {
    setContextWorkspaceId(workspaceId)
    setContextLists(lists)
    setSelectedList(undefined)
    setShouldRefreshOnClose(refreshOnClose)
    setDrawerVisible(true)
  }

  const openDrawerWithSelectedList = (
    workspaceId: string,
    lists: List[] = [],
    selectedList?: string,
    refreshOnClose = true
  ) => {
    setContextWorkspaceId(workspaceId)
    setContextLists(lists)
    setSelectedList(selectedList)
    setShouldRefreshOnClose(refreshOnClose)
    setDrawerVisible(true)
  }

  return (
    <CsvUploadContext.Provider value={{ openDrawer, openDrawerWithSelectedList }}>
      {children}
      {drawerVisible && (
        <ContactsCsvUploadDrawer
          workspaceId={contextWorkspaceId}
          lists={contextLists}
          selectedList={selectedList}
          onSuccess={handleImportSuccess}
          isVisible={drawerVisible}
          onClose={handleDrawerClose}
        />
      )}
    </CsvUploadContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components -- Hook co-located with provider
export function useContactsCsvUpload() {
  const context = React.useContext(CsvUploadContext)
  if (!context) {
    throw new Error('useContactsCsvUpload must be used within a ContactsCsvUploadDrawerProvider')
  }
  return context
}

export function ContactsCsvUploadButton() {
  const { t } = useLingui()
  const { openDrawer } = useContactsCsvUpload()

  return (
    <Button type="primary" onClick={() => openDrawer('')} icon={<UploadOutlined />}>
      {t`Import Contacts from CSV`}
    </Button>
  )
}
