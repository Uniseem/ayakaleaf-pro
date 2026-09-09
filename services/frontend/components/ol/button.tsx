'use client'

/**
 * The button, as the original's shared/components/button/button.tsx builds
 * it: a Bootstrap .btn with a variant and size class, an optional icon on
 * either side, and a loading state that keeps the button's width by hiding
 * the label under a spinner rather than replacing it.
 */

import { forwardRef, type ReactNode } from 'react'
import MaterialIcon from './material-icon'
import { Spinner } from './spinner'
import { useTranslation } from '@/lib/i18n'

export type ButtonVariant =
  | 'primary'
  | 'secondary'
  | 'ghost'
  | 'danger'
  | 'danger-ghost'
  | 'premium'
  | 'premium-secondary'
  | 'link'

export type ButtonProps = {
  children?: ReactNode
  className?: string
  disabled?: boolean
  download?: boolean | string
  draggable?: boolean
  form?: string
  leadingIcon?: string | ReactNode
  href?: string
  id?: string
  target?: string
  rel?: string
  isLoading?: boolean
  loadingLabel?: string
  onClick?: React.MouseEventHandler<HTMLButtonElement>
  onMouseDown?: React.MouseEventHandler<HTMLButtonElement>
  onMouseOver?: React.MouseEventHandler<HTMLButtonElement>
  onMouseOut?: React.MouseEventHandler<HTMLButtonElement>
  onMouseEnter?: React.MouseEventHandler<HTMLButtonElement>
  onMouseLeave?: React.MouseEventHandler<HTMLButtonElement>
  onFocus?: React.FocusEventHandler<HTMLButtonElement>
  onBlur?: React.FocusEventHandler<HTMLButtonElement>
  onKeyDown?: React.KeyboardEventHandler<HTMLButtonElement>
  size?: 'sm' | 'lg' | undefined
  style?: React.CSSProperties
  active?: boolean
  trailingIcon?: string | ReactNode
  type?: 'button' | 'reset' | 'submit'
  variant?: ButtonVariant
  'aria-label'?: string
  'aria-expanded'?: boolean
  'aria-haspopup'?: boolean | 'menu' | 'listbox' | 'dialog'
  'aria-controls'?: string
  'aria-pressed'?: boolean
  'aria-current'?: boolean
  'data-testid'?: string
  tabIndex?: number
  title?: string
  role?: string
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  function Button(
    {
      children,
      className,
      leadingIcon,
      isLoading = false,
      loadingLabel,
      trailingIcon,
      variant = 'primary',
      size,
      active,
      href,
      type = 'button',
      disabled,
      ...props
    },
    ref
  ) {
    const { t } = useTranslation()

    const buttonClassName = [
      'btn',
      `btn-${variant}`,
      size ? `btn-${size}` : '',
      active ? 'active' : '',
      'd-inline-grid',
      className ?? '',
      isLoading ? 'button-loading' : '',
      disabled && href ? 'disabled' : '',
    ]
      .filter(Boolean)
      .join(' ')

    const loadingSpinnerClassName =
      size === 'lg' ? 'loading-spinner-large' : 'loading-spinner-small'
    const materialIconClassName = size === 'lg' ? 'icon-large' : 'icon-small'

    const leadingIconComponent =
      leadingIcon && typeof leadingIcon === 'string' ? (
        <MaterialIcon type={leadingIcon} className={materialIconClassName} />
      ) : (
        leadingIcon
      )

    const trailingIconComponent =
      trailingIcon && typeof trailingIcon === 'string' ? (
        <MaterialIcon type={trailingIcon} className={materialIconClassName} />
      ) : (
        trailingIcon
      )

    const content = (
      <>
        {isLoading ? (
          <span className="spinner-container">
            <Spinner size="sm" className={loadingSpinnerClassName} />
            <span className="visually-hidden">
              {loadingLabel ?? t('loading')}
            </span>
          </span>
        ) : null}
        <span className="button-content" aria-hidden={isLoading}>
          {leadingIconComponent}
          {children}
          {trailingIconComponent}
        </span>
      </>
    )

    if (href) {
      const { onClick, target, rel, download, id, style, title } = props
      return (
        <a
          className={buttonClassName}
          href={href}
          target={target}
          rel={rel}
          download={download}
          id={id}
          style={style}
          title={title}
          aria-disabled={disabled || undefined}
          onClick={onClick as unknown as React.MouseEventHandler<HTMLAnchorElement>}
          data-ol-loading={isLoading}
        >
          {content}
        </a>
      )
    }

    return (
      <button
        className={buttonClassName}
        type={type}
        {...props}
        ref={ref}
        disabled={isLoading || disabled}
        data-ol-loading={isLoading}
      >
        {content}
      </button>
    )
  }
)

export default Button
