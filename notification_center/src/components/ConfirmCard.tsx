import { Button } from '@/components/ui/button'

interface ConfirmCardProps {
  // The list's name, or '' when the page does not know it (a private list the
  // contact is not on): the card then goes unnamed rather than showing an ID
  listName: string
  prompt: string
  buttonLabel: string
  processingLabel: string
  processing: boolean
  // 'danger' for leaving a list, 'primary' for joining one
  tone: 'danger' | 'primary'
  testId: string
  onConfirm: () => void
}

// The question a link from an email asks before anything is written: email security
// scanners open every link of an email in a real browser, so the click on this button
// is what tells a person from a scanner.
export function ConfirmCard({
  listName,
  prompt,
  buttonLabel,
  processingLabel,
  processing,
  tone,
  testId,
  onConfirm
}: ConfirmCardProps) {
  return (
    <div className="mb-6 p-4 rounded-sm border border-gray-300 bg-gray-50" data-testid={testId}>
      {listName && <div className="font-medium">{listName}</div>}
      <p className="mt-1 text-sm text-gray-700">{prompt}</p>
      <Button
        variant="outline"
        size="sm"
        onClick={onConfirm}
        disabled={processing}
        // The list's own row has a button with the same label
        aria-label={listName ? `${buttonLabel}: ${listName}` : buttonLabel}
        className={`mt-3 cursor-pointer ${
          tone === 'danger'
            ? 'border-red-500 text-red-500 hover:bg-red-50'
            : 'border-blue-500 text-blue-500 hover:bg-blue-50'
        }`}
      >
        {processing ? processingLabel : buttonLabel}
      </Button>
    </div>
  )
}
