'use client'

/**
 * A card, in Bootstrap's markup.
 *
 * The original uses `.card` on the pages outside the editor, so the ported
 * stylesheet already knows how to draw one: this is the markup that stylesheet
 * expects, and nothing more.
 */

import type { ReactNode } from 'react'
import cx from '@/lib/cx'

export function Card({
  children,
  className,
  id,
}: {
  children?: ReactNode
  className?: string
  id?: string
}) {
  return (
    <div className={cx('card', className)} id={id}>
      {children}
    </div>
  )
}

export function CardHeader({
  title,
  subtitle,
  children,
  className,
}: {
  title?: ReactNode
  subtitle?: ReactNode
  children?: ReactNode
  className?: string
}) {
  return (
    <div className={cx('card-header', className)}>
      {title ? <h2 className="card-title">{title}</h2> : null}
      {subtitle ? <p className="card-subtitle">{subtitle}</p> : null}
      {children}
    </div>
  )
}

export function CardBody({ children, className }: { children?: ReactNode; className?: string }) {
  return <div className={cx('card-body', className)}>{children}</div>
}

export default Card
