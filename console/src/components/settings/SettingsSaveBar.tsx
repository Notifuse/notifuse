import { useEffect, useRef } from 'react'
import { Button, Modal } from 'antd'
import { useBlocker } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'

interface SettingsSaveBarProps {
  /** True once the form holds edits that are not stored yet. */
  dirty: boolean
  /** True while the save request is in flight. */
  saving: boolean
  onSave: () => void
  onDiscard: () => void
  /** Body of the confirmation raised when leaving the section with edits pending. */
  leaveWarning: string
}

/**
 * The floating save affordance shared by the workspace settings sections.
 *
 * Render it as the LAST child of a section: a bottom-anchored sticky can shift
 * up across its whole containing block, so the bar rides the viewport for the
 * entire section and settles at the end. A `fixed` bar with a hardcoded left
 * offset would be wrong the moment the workspace sider collapses to 80px, which
 * is state these sections cannot read.
 *
 * It appears only once there is something to save — that pristine→dirty
 * transition is what makes it noticeable — and carries the Cmd/Ctrl+S shortcut
 * and the leave-the-page guard, so every settings form behaves the same way.
 */
export function SettingsSaveBar({
  dirty,
  saving,
  onSave,
  onDiscard,
  leaveWarning
}: SettingsSaveBarProps) {
  const { t } = useLingui()

  // The bar stays up through the save so the button can own the spinner; the
  // shortcut and the leave guard step aside once the request is in flight.
  const unsaved = dirty && !saving

  // Sections pass an inline handler, so a ref keeps the shortcut pointed at the
  // current one without rebinding the listener on every render.
  const onSaveRef = useRef(onSave)
  useEffect(() => {
    onSaveRef.current = onSave
  })

  // Cmd/Ctrl+S is what a long form trains people to reach for. Only claim the
  // shortcut while there is something to save, so the browser keeps it
  // otherwise.
  useEffect(() => {
    if (!unsaved) return
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
        event.preventDefault()
        onSaveRef.current()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [unsaved])

  // Switching settings section is a route change, so leaving with unsaved edits
  // is one click away and silent without this.
  const blocker = useBlocker({
    shouldBlockFn: () => unsaved,
    enableBeforeUnload: () => unsaved,
    withResolver: true
  })

  return (
    <>
      {dirty ? (
        <div className="sticky bottom-4 z-10 mt-8 flex justify-center">
          <div
            role="status"
            aria-live="polite"
            className="flex items-center gap-4 rounded-full border border-gray-200 bg-white/95 py-3 pl-7 pr-6 shadow-lg backdrop-blur"
          >
            <span className="text-sm text-gray-600">{t`You have unsaved changes`}</span>
            <Button size="small" onClick={onDiscard} disabled={saving}>
              {t`Discard`}
            </Button>
            <Button type="primary" size="small" loading={saving} onClick={onSave}>
              {t`Save Changes`}
            </Button>
          </div>
        </div>
      ) : null}

      <Modal
        open={blocker.status === 'blocked'}
        title={t`Discard unsaved changes?`}
        okText={t`Discard changes`}
        cancelText={t`Keep editing`}
        okButtonProps={{ danger: true }}
        onOk={() => blocker.proceed?.()}
        onCancel={() => blocker.reset?.()}
      >
        <p>{leaveWarning}</p>
      </Modal>
    </>
  )
}
