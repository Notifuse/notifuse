import { useCallback, useEffect, useState } from 'react'

const STORAGE_KEY = 'notifuse-editor-recent-colors'
const MAX_RECENT_COLORS = 8

export interface RecentColor {
  type: 'text' | 'background'
  value: string
  label: string
}

/**
 * Hook to manage recently used colors
 * Stores colors in localStorage and provides methods to add/retrieve them
 */
export function useRecentColors() {
  const [recentColors, setRecentColors] = useState<RecentColor[]>([])
  const [isInitialized, setIsInitialized] = useState(false)

  // Load recent colors from localStorage on mount
  useEffect(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY)
      if (stored) {
        const parsed = JSON.parse(stored) as RecentColor[]
        setRecentColors(parsed)
      }
    } catch (error) {
      console.error('Failed to load recent colors:', error)
    } finally {
      setIsInitialized(true)
    }
  }, [])

  // Add a color to recent colors
  const addRecentColor = useCallback((color: RecentColor) => {
    setRecentColors((prev) => {
      // Remove if already exists (to move to front)
      const filtered = prev.filter((c) => !(c.type === color.type && c.value === color.value))

      // Add to front and limit to MAX_RECENT_COLORS
      const updated = [color, ...filtered].slice(0, MAX_RECENT_COLORS)

      // Save to localStorage
      try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(updated))
      } catch (error) {
        console.error('Failed to save recent colors:', error)
      }

      return updated
    })
  }, [])

  // Clear all recent colors
  const clearRecentColors = useCallback(() => {
    setRecentColors([])
    try {
      localStorage.removeItem(STORAGE_KEY)
    } catch (error) {
      console.error('Failed to clear recent colors:', error)
    }
  }, [])

  return {
    recentColors,
    addRecentColor,
    clearRecentColors,
    isInitialized
  }
}
