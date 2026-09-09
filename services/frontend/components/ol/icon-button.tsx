'use client'

/**
 * A button that is only an icon, from shared/components/button/icon-button.
 *
 * The label goes on aria-label rather than on screen, and the padding is set
 * by size class so that the round hover shape is the same on every one.
 */

import { forwardRef } from 'react'
import MaterialIcon from './material-icon'
import { Button, type ButtonProps } from './button'
import type { AvailableUnfilledIcon } from '@/lib/unfilled-symbols'

type BaseIconButtonProps = ButtonProps & {
  accessibilityLabel?: string
  type?: 'button' | 'submit' | 'reset'
}

type FilledIconButtonProps = BaseIconButtonProps & {
  icon: string
  unfilled?: false
}

type UnfilledIconButtonProps = BaseIconButtonProps & {
  icon: AvailableUnfilledIcon
  unfilled: true
}

export type IconButtonProps = FilledIconButtonProps | UnfilledIconButtonProps

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  function IconButton(
    { accessibilityLabel, icon, isLoading = false, size, className, unfilled, ...props },
    ref
  ) {
    const iconButtonClassName = [
      className ?? '',
      !size ? 'icon-button' : '',
      size === 'sm' ? 'icon-button-small' : '',
      size === 'lg' ? 'icon-button-large' : '',
    ]
      .filter(Boolean)
      .join(' ')
    const iconSizeClassName = size === 'lg' ? 'icon-large' : 'icon-small'
    const materialIconClassName = [
      iconSizeClassName,
      isLoading ? 'button-content-hidden' : '',
      unfilled ? 'unfilled' : '',
    ]
      .filter(Boolean)
      .join(' ')

    return (
      <Button
        className={iconButtonClassName}
        isLoading={isLoading}
        aria-label={accessibilityLabel}
        size={size}
        {...props}
        ref={ref}
      >
        <MaterialIcon className={materialIconClassName} type={icon} />
      </Button>
    )
  }
)

export default IconButton
