import { useEffect, useState } from 'react'
import { App, Button, Form, Input, Popover } from 'antd'
import { Search } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { workspaceService } from '../../services/api/workspace'
import { WorkspaceBadge } from '../../layouts/app-shell/WorkspaceSwitcher'

interface LogoInputProps {
  /** Set by the Form.Item that holds the logo URL. */
  value?: string
  onChange?: (value: string) => void
  /** For the initial shown without a logo. */
  workspaceName?: string
  /** The field the website URL is read from by "Detect from website". */
  websiteField?: string
}

/**
 * The logo of a workspace: a 40px preview, "Replace" (a URL or the icon of the website) and
 * "Remove". It is the child of an always-mounted `Form.Item`: the popover mounts its content
 * only when first opened, and a field that is not mounted would empty the logo on save.
 */
export function LogoInput({
  value,
  onChange,
  workspaceName,
  websiteField = 'website_url'
}: LogoInputProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const form = Form.useFormInstance()
  const [isDetectingIcon, setIsDetectingIcon] = useState(false)
  const [open, setOpen] = useState(false)
  // The URL that failed to load, so the next one gets its chance.
  const [failedUrl, setFailedUrl] = useState<string | null>(null)
  const logoUrl = value || ''
  const failed = logoUrl !== '' && failedUrl === logoUrl

  useEffect(() => {
    if (failedUrl && failedUrl !== logoUrl) setFailedUrl(null)
  }, [logoUrl, failedUrl])

  const handleDetectIcon = async () => {
    const website = form.getFieldValue(websiteField)
    if (!website) {
      message.error(t`Please enter a website URL first`)
      return
    }

    setIsDetectingIcon(true)
    try {
      const { iconUrl } = await workspaceService.detectFavicon(website)

      if (iconUrl) {
        onChange?.(iconUrl)
        message.success(t`Icon detected successfully`)
      } else {
        message.warning(t`No icon found`)
      }
    } catch (error: unknown) {
      console.error('Error detecting icon:', error)
      message.error(t`Failed to detect icon: ${(error as Error).message || error}`)
    } finally {
      setIsDetectingIcon(false)
    }
  }

  const replace = (
    <div className="flex w-80 flex-col gap-2">
      <Input
        aria-label={t`Logo URL`}
        placeholder="https://example.com/logo.png"
        value={logoUrl}
        onChange={(event) => onChange?.(event.target.value)}
      />
      <Button
        icon={<Search size={14} aria-hidden="true" />}
        onClick={handleDetectIcon}
        loading={isDetectingIcon}
      >
        {t`Detect from website`}
      </Button>
    </div>
  )

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center gap-3">
        {logoUrl && !failed ? (
          <img
            src={logoUrl}
            alt={t`Logo Preview`}
            data-logo-preview
            onError={() => setFailedUrl(logoUrl)}
            className="size-10 shrink-0 rounded-md border border-line bg-surface object-contain"
          />
        ) : (
          <WorkspaceBadge workspace={{ name: workspaceName ?? '' }} large />
        )}
        <Popover
          trigger="click"
          placement="bottomLeft"
          open={open}
          onOpenChange={setOpen}
          content={replace}
        >
          <Button size="small">{t`Replace`}</Button>
        </Popover>
        {logoUrl ? (
          <Button size="small" type="text" danger onClick={() => onChange?.('')}>
            {t`Remove`}
          </Button>
        ) : null}
      </div>
      {failed ? (
        <div className="text-xs text-warning-fg">{t`This image couldn't be loaded`}</div>
      ) : null}
    </div>
  )
}
