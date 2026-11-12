import { useEffect, useRef, useState } from 'react'

const DELAY_MS = 300

/**
 * A search box kept in the URL: the box shows each keystroke at once, and `write` puts the
 * trimmed value in the URL 300 ms after the last one, so a typed word costs one request. Back,
 * forward and links change the box; the URL echoing our own write does not.
 */
export function useDebouncedUrlValue(
  urlValue: string,
  write: (value: string) => void
): [string, (value: string) => void] {
  const [input, setInput] = useState(urlValue)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const lastWritten = useRef<string | undefined>(undefined)
  const writeRef = useRef(write)
  useEffect(() => {
    writeRef.current = write
  })

  const onChange = (value: string) => {
    setInput(value)
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      const trimmed = value.trim()
      lastWritten.current = trimmed
      writeRef.current(trimmed)
    }, DELAY_MS)
  }

  useEffect(() => {
    if (urlValue !== lastWritten.current) {
      if (timer.current) clearTimeout(timer.current)
      // The URL is the outside value the box follows; the pending write must be cleared with it.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setInput(urlValue)
      lastWritten.current = urlValue
    }
  }, [urlValue])

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current)
    },
    []
  )

  return [input, onChange]
}
