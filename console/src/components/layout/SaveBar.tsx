import { useEffect, useRef } from 'react'
import { Button, Modal, Tooltip } from 'antd'
import { useBlocker } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'

export interface SaveBarProps {
  /** True once the form holds edits that are not stored yet. */
  dirty: boolean
  /** True while the save request is in flight. */
  saving: boolean
  onSave: () => void
  onDiscard: () => void
  /** Body of the confirmation raised when leaving the section with edits pending. */
  leaveWarning: string
  /** Label of the save button ("Save" by default). */
  saveLabel?: string
  /** The edits cannot be saved yet; the reason is the button's tooltip. */
  saveDisabled?: boolean
  saveDisabledReason?: string
  /**
   * Guards route changes with edits pending (default). Off for a screen that guards its own
   * leaving, so one confirmation is asked, not two.
   */
  blockNavigation?: boolean
}

function isMac(): boolean {
  return typeof navigator !== 'undefined' && /mac/i.test(navigator.platform)
}

/**
 * The floating save bar of a form (the workspace settings sections first).
 *
 * Render it as the LAST child of a section: a bottom-anchored sticky can shift
 * up across its whole containing block, so the bar rides the viewport for the
 * entire section and settles at the end. A `fixed` bar with a hardcoded left
 * offset would be wrong the moment the sidebar collapses, which is state these
 * sections cannot read.
 *
 * It appears only once there is something to save — that pristine→dirty
 * transition is what makes it noticeable — and carries the Cmd/Ctrl+S shortcut
 * and the leave-the-page guard, so every settings form behaves the same way.
 */
export function SaveBar({
  dirty,
  saving,
  onSave,
  onDiscard,
  leaveWarning,
  saveLabel,
  saveDisabled = false,
  saveDisabledReason,
  blockNavigation = true
}: SaveBarProps) {
  const { t } = useLingui()

  // The bar stays up through the save so the button can own the spinner; the
  // shortcut and the leave guard step aside once the request is in flight.
  const unsaved = dirty && !saving
  const canSave = unsaved && !saveDisabled

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
    if (!canSave) return
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
        event.preventDefault()
        onSaveRef.current()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [canSave])

  // Switching settings section is a route change, so leaving with unsaved edits
  // is one click away and silent without this.
  const blocker = useBlocker({
    shouldBlockFn: () => blockNavigation && unsaved,
    enableBeforeUnload: () => blockNavigation && unsaved,
    withResolver: true
  })

  return (
    <>
      {dirty ? (
        <div className="sticky bottom-3 z-10 mt-8">
          <div
            role="status"
            aria-live="polite"
            className="flex items-center justify-between gap-4 rounded-lg bg-ink py-2 pr-2 pl-4 text-sm text-white shadow-popup"
          >
            <span>{t`Unsaved changes`}</span>
            <div className="flex items-center gap-2">
              <Button ghost size="small" onClick={onDiscard} disabled={saving}>
                {t`Discard`}
              </Button>
              <Tooltip title={saveDisabled ? saveDisabledReason : isMac() ? '⌘S' : 'Ctrl+S'}>
                {/* A disabled button fires no mouse event: the span shows the reason. */}
                <span className="inline-flex">
                  <Button type="primary" size="small" loading={saving} disabled={saveDisabled} onClick={onSave}>
                    {saveLabel ?? t`Save`}
                  </Button>
                </span>
              </Tooltip>
            </div>
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
