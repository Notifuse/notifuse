import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'
import { HelpTip } from '../../../layout/HelpTip'
import { useLingui } from '@lingui/react/macro'

// Which sections are open, by key (block type + section), for the editor session: the panel
// remounts with each block, so the state lives in the editor.
interface SectionsState {
  isOpen: (key: string, firstSeen: boolean) => boolean
  setOpen: (key: string, open: boolean) => void
}

const SettingsSectionsContext = createContext<SectionsState | null>(null)

export function SettingsSectionsProvider({ children }: { children: ReactNode }) {
  const [open, setOpenState] = useState<Record<string, boolean>>({})
  // The state a key had the first time it showed, kept: a section the user never toggled does not
  // open and close as its fields are set and reset
  const firstSeenByKey = useRef<Record<string, boolean>>({})
  const isOpen = useCallback(
    (key: string, firstSeen: boolean) => {
      if (!(key in firstSeenByKey.current)) firstSeenByKey.current[key] = firstSeen
      return open[key] ?? firstSeenByKey.current[key]
    },
    [open]
  )
  const setOpen = useCallback((key: string, value: boolean) => setOpenState((prev) => ({ ...prev, [key]: value })), [])
  const value = useMemo(() => ({ isOpen, setOpen }), [isOpen, setOpen])
  return <SettingsSectionsContext.Provider value={value}>{children}</SettingsSectionsContext.Provider>
}

// Without the editor (a test, a panel rendered alone), each section keeps its own state.
function useSectionOpen(sectionKey: string, firstSeen: boolean): [boolean, (open: boolean) => void] {
  const shared = useContext(SettingsSectionsContext)
  const [local, setLocal] = useState(firstSeen)
  if (shared) return [shared.isOpen(sectionKey, firstSeen), (open) => shared.setOpen(sectionKey, open)]
  return [local, setLocal]
}

export interface SettingsSectionProps {
  /** Block type + section, e.g. "mj-button:font": the open state is kept per key. */
  sectionKey: string
  title: string
  help?: ReactNode
  /** Shown in the header when the section is closed. */
  summary?: ReactNode
  /** Fields set on the block: counted in the closed header; a section with one opens the first time. */
  setCount?: number
  /** The block's first section opens the first time too. */
  first?: boolean
  children: ReactNode
}

// A collapsible group of settings: closed, it shows a summary of its values and how many are set.
export function SettingsSection({ sectionKey, title, help, summary, setCount = 0, first = false, children }: SettingsSectionProps) {
  const { t } = useLingui()
  const [open, setOpen] = useSectionOpen(sectionKey, first || setCount > 0)
  const bodyId = `settings-section-${sectionKey.replace(/[^a-z0-9-]/gi, '-')}`
  return (
    // Open or closed, a section is a white card on the panel's grey; open, a rule parts its title
    // from its fields
    <section className="m-2 rounded-lg border border-line bg-surface" data-testid="settings-section">
      <div className={`flex min-h-9 items-center gap-2 px-3 ${open ? 'border-b border-line-soft' : ''}`}>
        <button
          type="button"
          aria-expanded={open}
          aria-controls={bodyId}
          onClick={() => setOpen(!open)}
          className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 border-0 bg-transparent p-0 py-2.5 text-left"
        >
          <ChevronDown size={12} aria-hidden="true" className={`flex-none text-ink-2 transition-transform ${open ? '' : '-rotate-90'}`} />
          <span className="text-overline font-semibold tracking-wider text-ink uppercase">{title}</span>
          {!open && setCount > 0 ? (
            <span
              className="min-w-3.25 rounded-full bg-primary-100 px-0.75 text-center text-[9px]/[13px] font-semibold text-primary-700"
              aria-label={t`${setCount} set`}
            >
              {setCount}
            </span>
          ) : null}
          {!open && summary ? <span className="ml-auto min-w-0 truncate text-2xs text-ink-3">{summary}</span> : null}
        </button>
        {help ? (
          <HelpTip label={t`About ${title}`} icon="info" placement="left">
            {help}
          </HelpTip>
        ) : null}
      </div>
      {open ? (
        <div id={bodyId} className="px-3 pt-2.5 pb-3">
          {children}
        </div>
      ) : null}
    </section>
  )
}
