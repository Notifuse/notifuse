import { useEffect, useRef, useState, type ReactNode } from 'react'

/**
 * Renders its children once they scroll into view, so a long list compiles only the previews
 * shown. Without IntersectionObserver (tests, old browsers) it renders them at once.
 */
export function LazyVisible({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)
  const [visible, setVisible] = useState(typeof IntersectionObserver === 'undefined')
  useEffect(() => {
    if (visible || !ref.current) return
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting) {
        setVisible(true)
        observer.disconnect()
      }
    })
    observer.observe(ref.current)
    return () => observer.disconnect()
  }, [visible])
  return (
    <div ref={ref} className="h-full w-full">
      {visible ? children : null}
    </div>
  )
}
