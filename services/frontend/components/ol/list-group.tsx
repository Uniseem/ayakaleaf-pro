'use client'

/** Bootstrap's list group, as the toolbar menus use it. */

import { forwardRef, type ReactNode } from 'react'
import cx from '@/lib/cx'
import { Tooltip } from './tooltip'

export type ListGroupProps = React.HTMLAttributes<HTMLDivElement> & {
  role?: string
  children: ReactNode
}

export const ListGroup = forwardRef<HTMLDivElement, ListGroupProps>(function ListGroup({ className, children, ...props }, ref) {
  return (
    <div ref={ref} className={cx('list-group', className)} {...props}>
      {children}
    </div>
  )
})

export type ListGroupItemProps = Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'disabled'> & {
  active?: boolean
  disabled?: boolean
  /** Shown as a tooltip when the item is disabled. */
  disabledReason?: string
  children: ReactNode
}

export const ListGroupItem = forwardRef<HTMLButtonElement, ListGroupItemProps>(function ListGroupItem(
  { className, active, disabled, disabledReason, children, onClick, ...props },
  ref
) {
  const button = (
    <button
      ref={ref}
      type="button"
      className={cx('list-group-item list-group-item-action', { active, disabled }, className)}
      aria-disabled={disabled || undefined}
      onClick={disabled ? undefined : onClick}
      {...props}
    >
      {children}
    </button>
  )

  if (disabled && disabledReason) {
    return (
      <Tooltip id={`${props.id ?? 'list-group-item'}-disabled`} description={disabledReason} overlayProps={{ placement: 'bottom' }}>
        <span className="d-inline-block w-100">{button}</span>
      </Tooltip>
    )
  }

  return button
})

export function ButtonGroup({ className, children, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div role="group" className={cx('btn-group', className)} {...props}>
      {children}
    </div>
  )
}
