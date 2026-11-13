import { Button, Modal } from 'antd'
import { TriangleAlert } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'

interface ImportErrorsModalProps {
  open: boolean
  title: string
  errors: string[]
  onClose: () => void
}

/** The validation errors of an imported template file, shown by the editor and the page. */
export function ImportErrorsModal({ open, title, errors, onClose }: ImportErrorsModalProps) {
  const { t } = useLingui()
  return (
    <Modal
      title={
        <div className="flex items-center gap-2">
          <TriangleAlert size={16} aria-hidden="true" className="text-danger-fg" />
          <span>{title}</span>
        </div>
      }
      open={open}
      onCancel={onClose}
      footer={[
        <Button key="ok" type="primary" onClick={onClose}>
          {t`OK`}
        </Button>
      ]}
      width={600}
    >
      <div className="flex flex-col gap-3">
        <p className="text-ink-2">{t`The following validation errors were found in your import:`}</p>

        <div className="max-h-60 overflow-y-auto rounded-md border border-danger-fg/25 bg-danger-bg p-3">
          <ul className="flex flex-col gap-2">
            {errors.map((error, index) => (
              <li key={index} className="flex items-start gap-2">
                <span className="mt-0.5 font-bold text-danger-fg">•</span>
                <span className="whitespace-pre-line text-sm text-danger-fg">{error}</span>
              </li>
            ))}
          </ul>
        </div>

        <div className="text-sm text-ink-3">
          <p className="font-medium">{t`Tips for fixing these errors:`}</p>
          <ul className="mt-1 ml-4 flex flex-col gap-1">
            <li>{t`• Check that your MJML structure follows the proper hierarchy`}</li>
            <li>{t`• Ensure all required components are properly nested`}</li>
            <li>{t`• Verify that component attributes are valid`}</li>
            <li>{t`• Make sure all XML tags are properly closed`}</li>
          </ul>
        </div>
      </div>
    </Modal>
  )
}
