import { memo, type ComponentProps } from 'react'
import type { AvailableUnfilledIcon } from '@/lib/unfilled-symbols'

/**
 * A Material Symbols icon, by ligature name.
 *
 * The same component the original uses, with the same two faces: filled by
 * default, and `unfilled` for the outline face -- which only exists for the
 * names in the typed list, hence the two prop shapes.
 */

type BaseIconProps = ComponentProps<'span'> & {
  accessibilityLabel?: string
  modifier?: string
  size?: '2x'
}

type FilledIconProps = BaseIconProps & {
  type: string
  unfilled?: false
}

type UnfilledIconProps = BaseIconProps & {
  type: AvailableUnfilledIcon
  unfilled: true
}

export type IconProps = FilledIconProps | UnfilledIconProps

function MaterialIcon({
  type,
  className,
  accessibilityLabel,
  modifier,
  size,
  unfilled,
  ...rest
}: IconProps) {
  const iconClassName = [
    'material-symbols',
    className,
    modifier,
    size ? `size-${size}` : '',
    unfilled ? 'unfilled' : '',
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <>
      <span
        className={iconClassName}
        aria-hidden="true"
        translate="no"
        {...rest}
      >
        {type}
      </span>
      {accessibilityLabel ? (
        <span className="visually-hidden">{accessibilityLabel}</span>
      ) : null}
    </>
  )
}

export default memo(MaterialIcon)
