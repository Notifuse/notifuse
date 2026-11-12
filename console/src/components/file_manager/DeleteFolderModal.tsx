import { Modal } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { folderName } from './storageTree'

interface DeleteFolderModalProps {
  /** The folder to delete ("docs/"), null while closed. */
  folder: string | null
  workspaceId: string
  bucket: string
  /** Reads the folder's own listing, not the cached one: it may be capped or stale. */
  countFiles: (folder: string) => Promise<number>
  loading: boolean
  zIndex?: number
  onConfirm: (folder: string) => void
  onCancel: () => void
}

/** Confirms a folder delete with the number of files it holds. */
export function DeleteFolderModal({
  folder,
  workspaceId,
  bucket,
  countFiles,
  loading,
  zIndex,
  onConfirm,
  onCancel
}: DeleteFolderModalProps) {
  const { t } = useLingui()
  const count = useQuery({
    queryKey: ['storage-folder-count', workspaceId, bucket, folder],
    queryFn: () => countFiles(folder as string),
    enabled: folder !== null,
    staleTime: 0,
    gcTime: 0
  })

  const name = folder ? folderName(folder) : ''
  const question =
    count.data === undefined
      ? t`Delete the folder ${name} and its files?`
      : plural(count.data, {
          one: `Delete the folder ${name} and its # file?`,
          other: `Delete the folder ${name} and its # files?`
        })

  return (
    <Modal
      open={folder !== null}
      title={t`Delete folder`}
      okText={t`Delete folder`}
      cancelText={t`Cancel`}
      okButtonProps={{ danger: true, loading }}
      zIndex={zIndex}
      onOk={() => folder && onConfirm(folder)}
      onCancel={onCancel}
      destroyOnHidden
    >
      <p>{question}</p>
      <p className="text-ink-3">{t`Images used in emails or blog posts will stop displaying.`}</p>
    </Modal>
  )
}
