import type { ReactNode } from 'react'
import { Link, type LinkProps } from '@tanstack/react-router'

export type MenuTarget = Pick<LinkProps, 'to' | 'params'>

/** A sidebar entry's label: a router link to its page. */
export function MenuLink({ target, children }: { target: MenuTarget; children: ReactNode }) {
  return <Link {...target}>{children}</Link>
}
