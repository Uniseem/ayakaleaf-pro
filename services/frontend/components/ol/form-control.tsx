'use client'

/**
 * Bootstrap's form control, from shared/components/form/form-control.tsx:
 * an input with an optional icon at either end, kept inside a wrapper so
 * the icon sits over the field's padding.
 */

import { forwardRef, type ReactNode } from 'react'
import { Spinner } from './spinner'
import cx from '@/lib/cx'

export type FormControlProps = Omit<React.ComponentProps<'input'>, 'size' | 'prefix'> & {
  prepend?: ReactNode
  append?: ReactNode
  size?: 'sm' | 'lg'
  /**
   * The input's own `size` attribute, in characters. Named apart from `size`
   * because that one is the control's variant here, as in the original.
   */
  htmlSize?: number
  isInvalid?: boolean
  isValid?: boolean
  loading?: boolean
  as?: 'input' | 'textarea'
  rows?: number
}

export const FormControl = forwardRef<HTMLInputElement, FormControlProps>(function FormControl(
  { prepend, append, className, size, htmlSize, isInvalid, isValid, loading, as = 'input', rows, ...props },
  ref
) {
  const controlClassName = [
    'form-control',
    size ? `form-control-${size}` : '',
    isInvalid ? 'is-invalid' : '',
    isValid ? 'is-valid' : '',
    prepend ? 'form-control-offset-start' : '',
    append || loading ? 'form-control-offset-end' : '',
    className ?? '',
  ]
    .filter(Boolean)
    .join(' ')

  const control =
    as === 'textarea' ? (
      <textarea
        ref={ref as unknown as React.Ref<HTMLTextAreaElement>}
        className={controlClassName}
        rows={rows}
        {...(props as unknown as React.ComponentProps<'textarea'>)}
      />
    ) : (
      <input ref={ref} className={controlClassName} size={htmlSize} {...props} />
    )

  const end = loading ? <Spinner size="sm" /> : append

  if (prepend || end) {
    return (
      <div
        className={[
          'form-control-wrapper',
          size === 'sm' ? 'form-control-wrapper-sm' : '',
          size === 'lg' ? 'form-control-wrapper-lg' : '',
          props.disabled ? 'form-control-wrapper-disabled' : '',
        ]
          .filter(Boolean)
          .join(' ')}
      >
        {prepend ? <span className="form-control-start-icon">{prepend}</span> : null}
        {control}
        {end ? <span className="form-control-end-icon">{end}</span> : null}
      </div>
    )
  }

  return control
})

export const OLFormControl = FormControl

export function OLFormGroup({
  children,
  className,
  controlId,
}: {
  children: ReactNode
  className?: string
  controlId?: string
}) {
  return (
    <div className={['form-group', className ?? ''].filter(Boolean).join(' ')} data-control-id={controlId}>
      {children}
    </div>
  )
}

export function OLFormLabel({
  children,
  className,
  htmlFor,
  id,
}: {
  children: ReactNode
  className?: string
  htmlFor?: string
  id?: string
}) {
  return (
    <label id={id} htmlFor={htmlFor} className={['form-label', className ?? ''].filter(Boolean).join(' ')}>
      {children}
    </label>
  )
}

export function OLFormText({
  children,
  className,
  id,
  type,
}: {
  children: ReactNode
  className?: string
  id?: string
  type?: 'default' | 'info' | 'success' | 'warning' | 'error'
}) {
  return (
    <div id={id} className={['form-text', type ? `form-text-${type}` : '', className ?? ''].filter(Boolean).join(' ')}>
      <span className="form-text-inner">{children}</span>
    </div>
  )
}

export function OLFormFeedback({
  children,
  className,
  type = 'invalid',
}: {
  children: ReactNode
  className?: string
  type?: 'valid' | 'invalid'
  unfilled?: boolean
}) {
  return <div className={[`${type}-feedback`, className ?? ''].filter(Boolean).join(' ')}>{children}</div>
}

export default FormControl

export type OLFormCheckboxProps = Omit<React.ComponentProps<'input'>, 'type'> & {
  type?: 'checkbox' | 'radio'
  label?: ReactNode
  description?: string
  inputRef?: React.RefObject<HTMLInputElement | null>
}

/**
 * A checkbox or radio with its label.
 *
 * The label is a node rather than a string because callers put a description
 * underneath it, and it has to stay inside the label element so that clicking
 * the description still toggles the control.
 */
export function OLFormCheckbox({
  id,
  type = 'checkbox',
  label,
  description,
  className,
  inputRef,
  ...props
}: OLFormCheckboxProps) {
  const describedBy = description && id ? `${id}-description` : undefined

  return (
    <div className={cx('form-check', type === 'checkbox' && 'form-checkbox', className)}>
      <input
        ref={inputRef}
        id={id}
        type={type}
        className="form-check-input"
        aria-describedby={describedBy}
        {...props}
      />
      {label !== undefined && (
        <label className="form-check-label" htmlFor={id}>
          {label}
          {description && (
            <OLFormText id={describedBy} className="form-check-label-description">
              {description}
            </OLFormText>
          )}
        </label>
      )}
    </div>
  )
}
