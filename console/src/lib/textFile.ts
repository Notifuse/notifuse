// Minimal helpers for importing/exporting a plain-text field as a .txt file.
// Kept separate from the MJML/JSON import-export logic in ImportExportButton.tsx and
// MjmlCodeEditor.tsx, which validate structure — plain text has none to validate.

export const readTextFile = (file: File): Promise<string> => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = (e) => resolve((e.target?.result as string) ?? '')
    reader.onerror = () => reject(reader.error)
    reader.readAsText(file)
  })
}

export const downloadTextFile = (content: string, filename: string): void => {
  const blob = new Blob([content], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}
