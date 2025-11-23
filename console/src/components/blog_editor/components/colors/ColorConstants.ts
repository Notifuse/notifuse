/**
 * Shared color palettes for text and background colors
 * Used across toolbar and block actions menu
 */

export interface ColorOption {
  label: string
  value: string | null
}

// Text color palette
export const TEXT_COLORS: ColorOption[] = [
  { label: 'Default', value: null },
  { label: 'Gray', value: 'hsl(45, 2%, 46%)' },
  { label: 'Brown', value: 'hsl(19, 31%, 47%)' },
  { label: 'Orange', value: 'hsl(30, 89%, 45%)' },
  { label: 'Yellow', value: 'hsl(38, 62%, 49%)' },
  { label: 'Green', value: 'hsl(148, 32%, 39%)' },
  { label: 'Blue', value: 'hsl(202, 54%, 43%)' },
  { label: 'Purple', value: 'hsl(274, 32%, 54%)' },
  { label: 'Pink', value: 'hsl(328, 49%, 53%)' },
  { label: 'Red', value: 'hsl(2, 62%, 55%)' }
]

// Background color palette
export const BACKGROUND_COLORS: ColorOption[] = [
  { label: 'Default', value: null },
  { label: 'Gray', value: 'rgb(248, 248, 247)' },
  { label: 'Brown', value: 'rgb(244, 238, 238)' },
  { label: 'Orange', value: 'rgb(251, 236, 221)' },
  { label: 'Yellow', value: '#fef9c3' },
  { label: 'Green', value: '#dcfce7' },
  { label: 'Blue', value: '#e0f2fe' },
  { label: 'Purple', value: '#f3e8ff' },
  { label: 'Pink', value: 'rgb(252, 241, 246)' },
  { label: 'Red', value: '#ffe4e6' }
]
