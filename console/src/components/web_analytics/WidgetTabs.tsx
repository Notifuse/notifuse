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
 * Deliberately not AntD's `Tabs`: that component owns the panel below it, and
 * here the card's body is a table the widget renders itself. It takes the antd
 * `Tabs` look from the theme instead (13px labels, 18px gutter, ink-3 → ink,
 * primary-500 bar) and the panel's inline padding: a row of buttons over a
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
    <div role="tablist" className={`flex gap-4.5 border-b border-line-soft px-3.5 ${className ?? ''}`}>
      {tabs.map((tab) => {
        const active = tab.key === activeKey
        return (
          <button
            key={tab.key}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(tab.key)}
            className={`-mb-px cursor-pointer border-b-2 py-2 text-base transition-colors ${
              active ? 'border-primary-500 text-ink' : 'border-transparent text-ink-3 hover:text-ink-2'
            }`}
          >
            {tab.label}
          </button>
        )
      })}
    </div>
  )
}
