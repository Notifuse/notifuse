export interface WidgetTab {
  key: string
  label: string
}

interface WidgetTabsProps {
  tabs: WidgetTab[]
  activeKey: string
  onChange: (key: string) => void
  className?: string
}

/**
 * The tab strip inside an analytics card.
 *
 * Deliberately not AntD's `Tabs`: that component owns the panel below it and
 * brings its own ink bar, padding and motion, none of which fit a card whose
 * body is a table the widget renders itself. This is a row of buttons over a
 * hairline, with the active tab's thicker underline pulled down a pixel to sit
 * on top of it — which is why the container carries the rule rather than each
 * button.
 *
 * One component rather than markup repeated per widget: a card and its expanded
 * drawer each need a strip, and two of them drifting apart is exactly how a
 * dashboard ends up with two tab styles side by side.
 */
export function WidgetTabs({ tabs, activeKey, onChange, className }: WidgetTabsProps) {
  if (tabs.length <= 1) return null

  return (
    <div className={`flex gap-4 border-b border-gray-200 px-4 ${className ?? ''}`}>
      {tabs.map((tab) => (
        <button
          key={tab.key}
          type="button"
          onClick={() => onChange(tab.key)}
          className={`-mb-px cursor-pointer border-b-2 pb-2 text-xs transition-colors ${
            tab.key === activeKey
              ? 'border-[var(--primary)] text-gray-900'
              : 'border-transparent text-gray-500 hover:text-gray-700'
          }`}
        >
          {tab.label}
        </button>
      ))}
    </div>
  )
}
