import { useState, useCallback, type ReactNode } from 'react'
import { Modal, Button } from 'antd'
import { useLingui } from '@lingui/react/macro'

export interface ConflictInfo {
  /** The current server version, when the 409 body reports it. */
  latestVersion?: number
  /** The revision the local edits were based on. */
  baseVersion?: number
  /** Force-save the local content over the server's version. */
  onOverwrite: () => void
  /** Discard local edits and load the server's latest version. */
  onReload: () => void
}

/**
 * Shared conflict dialog for template editors. Presented when a save is rejected because
 * the template advanced past the edited revision (HTTP 409).
 *
 * The two resolution actions ("Reload latest" discards work, "Overwrite with mine" forces
 * the save) require an explicit button click. Dismissing the dialog — via the "Keep
 * editing" button, the close icon, or ESC — is a safe no-op that preserves the user's
 * in-progress work, so a reflexive ESC never destroys edits.
 */
export function useTemplateConflictModal(): {
  conflictModal: ReactNode
  showConflict: (info: ConflictInfo) => void
} {
  const { t } = useLingui()
  const [info, setInfo] = useState<ConflictInfo | null>(null)

  const close = useCallback(() => setInfo(null), [])
  const showConflict = useCallback((next: ConflictInfo) => setInfo(next), [])

  const conflictModal = (
    <Modal
      open={info !== null}
      title={t`Template changed by someone else`}
      onCancel={close}
      maskClosable={false}
      footer={[
        <Button key="keep" onClick={close}>
          {t`Keep editing`}
        </Button>,
        <Button
          key="reload"
          danger
          onClick={() => {
            info?.onReload()
            close()
          }}
        >
          {t`Reload latest`}
        </Button>,
        <Button
          key="overwrite"
          type="primary"
          onClick={() => {
            info?.onOverwrite()
            close()
          }}
        >
          {t`Overwrite with mine`}
        </Button>
      ]}
    >
      {info?.latestVersion ? (
        <p>{t`Someone saved version ${info.latestVersion} while you were editing (you started from ${info.baseVersion ?? 0}). Reload the latest version — your unsaved changes will be lost — or overwrite it with yours.`}</p>
      ) : (
        <p>{t`Someone changed this template while you were editing. Reload the latest version — your unsaved changes will be lost — or overwrite it with yours.`}</p>
      )}
    </Modal>
  )

  return { conflictModal, showConflict }
}
