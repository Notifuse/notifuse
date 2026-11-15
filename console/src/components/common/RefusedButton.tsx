import { useId, type MouseEvent } from 'react'
import { Button, Tooltip, type ButtonProps } from 'antd'

export interface RefusedButtonProps extends Omit<ButtonProps, 'disabled'> {
  // Why the button cannot act: undefined when it can, empty while the reason is still loading
  refusal?: string
  // The tooltip of the button when it can act, such as the name of an icon-only button
  tooltip?: string
}

/**
 * A button that a refusal makes inert without taking it out of the tab order: `aria-disabled`
 * rather than `disabled`, so the keyboard reaches it and a screen reader reads why
 * (`aria-describedby`). antd's own `ant-btn-disabled` class, the one it gives a disabled link
 * button, draws it disabled and drops its hover; an enabled button receives the hover that
 * shows the tooltip, which a disabled one would not.
 */
export function RefusedButton({ refusal, tooltip, onClick, className, ...rest }: RefusedButtonProps) {
  const reasonId = useId()
  const refused = refusal !== undefined
  const classes = [className, refused ? 'ant-btn-disabled' : undefined].filter(Boolean).join(' ')
  const click = (event: MouseEvent<HTMLElement>) => {
    if (refused) {
      event.preventDefault()
      return
    }
    onClick?.(event)
  }
  return (
    <>
      <Tooltip title={refusal || tooltip}>
        {/* A wrapper: the tooltip sets aria-describedby on its child, and would drop the button's own */}
        <span className="inline-flex">
          <Button
            {...rest}
            className={classes || undefined}
            aria-disabled={refused || undefined}
            aria-describedby={refusal ? reasonId : undefined}
            onClick={click}
          />
        </span>
      </Tooltip>
      {refusal ? (
        <span id={reasonId} className="sr-only">
          {refusal}
        </span>
      ) : null}
    </>
  )
}
