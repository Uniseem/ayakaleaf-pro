'use client'

/**
 * A row of buttons behaving as one radio group, from react-bootstrap's
 * ToggleButtonGroup.
 *
 * Real radio inputs under the buttons rather than buttons with aria-pressed:
 * a radio group is what this is, and a keyboard user gets arrow-key movement
 * between the options for free.
 */

import { createContext, useContext, type ReactNode } from 'react'
import cx from '@/lib/cx'

type GroupContext = {
  name: string
  value: string | undefined
  onChange: (value: string) => void
}

const ToggleButtonGroupContext = createContext<GroupContext | undefined>(undefined)

export type ToggleButtonGroupProps = {
  name: string
  defaultValue?: string
  value?: string
  onChange: (value: string) => void
  className?: string
  children: ReactNode
  'aria-label'?: string
}

export function OLToggleButtonGroup({
  name,
  defaultValue,
  value,
  onChange,
  className,
  children,
  ...rest
}: ToggleButtonGroupProps) {
  return (
    <ToggleButtonGroupContext.Provider
      value={{ name, value: value ?? defaultValue, onChange }}
    >
      <div role="radiogroup" className={cx('btn-group', className)} {...rest}>
        {children}
      </div>
    </ToggleButtonGroupContext.Provider>
  )
}

export type ToggleButtonProps = {
  id: string
  value: string
  variant?: 'primary' | 'secondary'
  disabled?: boolean
  className?: string
  children: ReactNode
}

export function OLToggleButton({
  id,
  value,
  variant = 'secondary',
  disabled,
  className,
  children,
}: ToggleButtonProps) {
  const group = useContext(ToggleButtonGroupContext)
  if (!group) {
    throw new Error('OLToggleButton is only available inside OLToggleButtonGroup')
  }
  const checked = group.value === value

  return (
    <>
      <input
        type="radio"
        className="btn-check"
        id={id}
        name={group.name}
        value={value}
        checked={checked}
        disabled={disabled}
        onChange={() => group.onChange(value)}
      />
      <label
        className={cx('btn', `btn-${variant}`, checked && 'active', className)}
        htmlFor={id}
      >
        {children}
      </label>
    </>
  )
}
