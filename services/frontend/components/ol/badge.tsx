'use client'

/**
 * Bootstrap badges and the tag built on them, from shared/components/badge
 * and shared/components/tag.tsx.
 */

import { forwardRef, type ReactNode } from 'react'
import MaterialIcon from './material-icon'
import { useTranslation } from '@/lib/i18n'

type Bg = 'primary' | 'secondary' | 'success' | 'info' | 'warning' | 'warning-light-bg' | 'danger' | 'light' | 'dark'

export type BadgeProps = {
  bg?: Bg
  text?: 'dark' | 'light' | 'warning'
  pill?: boolean
  className?: string
  prepend?: ReactNode
  children?: ReactNode
  badgeContentRef?: React.RefObject<HTMLElement | null>
}

export const Badge = forwardRef<HTMLSpanElement, BadgeProps>(function Badge(
  { bg = 'primary', text, pill, className, prepend, children, badgeContentRef },
  ref
) {
  return (
    <span
      ref={ref}
      className={['badge', `bg-${bg}`, text ? `text-${text}` : '', pill ? 'rounded-pill' : '', className ?? '']
        .filter(Boolean)
        .join(' ')}
    >
      {prepend ? <span className="badge-prepend">{prepend}</span> : null}
      <span className="badge-content" ref={badgeContentRef as React.RefObject<HTMLSpanElement>}>
        {children}
      </span>
    </span>
  )
})

/** The original's OLBadge: a warning badge gets a light ground and dark text. */
export function OLBadge(props: BadgeProps) {
  let { bg, text } = props
  if (bg === 'warning') {
    bg = 'warning-light-bg'
    text = 'warning'
  }
  return <Badge {...props} bg={bg} text={text} />
}

export type TagProps = {
  prepend?: ReactNode
  contentProps?: React.ComponentProps<'button'>
  closeBtnProps?: React.ComponentProps<'button'>
  className?: string
  children?: ReactNode
  translate?: 'yes' | 'no'
  bg?: Bg
}

export const Tag = forwardRef<HTMLSpanElement, TagProps & Omit<React.HTMLAttributes<HTMLSpanElement>, keyof TagProps>>(function Tag(
  { prepend, children, contentProps, closeBtnProps, className, bg = 'light', translate, ...rest },
  ref
) {
  const { t } = useTranslation()

  const content = (
    <>
      {prepend ? <span className="badge-prepend">{prepend}</span> : null}
      <span className="badge-content">{children}</span>
    </>
  )

  return (
    <span
      ref={ref}
      className={['badge', `bg-${bg}`, 'badge-tag', className ?? ''].filter(Boolean).join(' ')}
      translate={translate}
      {...rest}
    >
      {contentProps?.onClick ? (
        <button
          type="button"
          {...contentProps}
          className={['badge-tag-content badge-tag-content-btn', contentProps.className ?? ''].filter(Boolean).join(' ')}
        >
          {content}
        </button>
      ) : (
        <span {...(contentProps as React.ComponentProps<'span'>)} className={['badge-tag-content', contentProps?.className ?? ''].filter(Boolean).join(' ')}>
          {content}
        </span>
      )}
      {closeBtnProps ? (
        <button
          type="button"
          className="badge-close"
          aria-label={t('remove_tag', { tagName: typeof children === 'string' ? children : '' })}
          {...closeBtnProps}
        >
          <MaterialIcon className="badge-close-icon" type="close" />
        </button>
      ) : null}
    </span>
  )
})

export default Badge
