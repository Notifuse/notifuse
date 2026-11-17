import { forwardRef } from 'react'
import { Button, type ButtonProps } from 'antd'
import { createLink } from '@tanstack/react-router'

interface ButtonLinkHostProps extends Omit<ButtonProps, 'type'> {
  /** The page's one primary action. Not antd's `type`: the router's links never pass `type` on. */
  primary?: boolean
}

/**
 * An antd button that navigates through the router: antd renders an <a> for an href, so it is a
 * real link (open in a new tab works) without a button nested in a link.
 */
export const ButtonLink = createLink(
  forwardRef<HTMLAnchorElement, ButtonLinkHostProps>(({ primary, ...props }, ref) => (
    <Button ref={ref as never} type={primary ? 'primary' : 'default'} {...props} />
  ))
)
